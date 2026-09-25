package ui

// download.go — lógica de descarga dentro del TUI usando Bubble Tea Cmds.
// Permite mostrar barra de progreso en tiempo real en downloadScreen.

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"

	tea "github.com/charmbracelet/bubbletea"

	"mcTUI/internal/config"
	"mcTUI/internal/fabric"
	"mcTUI/internal/mojang"
)

// StartDownloadCmd arranca la descarga de los archivos faltantes en una
// goroutine y va enviando DownloadProgressMsg al TUI a medida que cada
// archivo termina. Al finalizar envía DownloadDoneMsg.
func StartDownloadCmd(version, modloader string) tea.Cmd {
	return func() tea.Msg {
		// Este Cmd lanza la descarga y retorna el primer mensaje.
		// Los mensajes subsiguientes son enviados vía el canal prog.
		// Como tea.Cmd solo puede retornar UN mensaje, usamos el patron
		// de retornar el primer DownloadProgressMsg y luego los demás
		// son enviados como Cmds encadenados desde Update().
		//
		// En realidad aquí devolvemos directamente el primer msg,
		// y cada DownloadProgressMsg hace que Update() devuelva
		// otro Cmd que espera el siguiente mensaje del canal.
		// Usamos un channel buffered global por sesión.
		ch := startDownload(version, modloader)
		msg := <-ch
		return downloadChanMsg{ch: ch, msg: msg}
	}
}

// downloadChanMsg es un mensaje interno que porta el canal y el mensaje
// actual — permite encadenar la recepción sin polling.
type downloadChanMsg struct {
	ch  <-chan interface{}
	msg interface{}
}

// nextDownloadCmd devuelve un Cmd que espera el próximo mensaje del canal.
func nextDownloadCmd(ch <-chan interface{}) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return DownloadDoneMsg{Err: fmt.Errorf("download stopped without a result")}
		}
		return downloadChanMsg{ch: ch, msg: msg}
	}
}

// startDownload lanza la descarga real en goroutines y retorna un canal
// de mensajes (DownloadProgressMsg y DownloadDoneMsg) para el TUI.
func startDownload(version, modloader string) <-chan interface{} {
	ch := make(chan interface{}, 64)

	go func() {
		defer close(ch)

		mcDir := config.MinecraftDir()

		// Contar cuántos archivos hay que descargar para el total.
		var totalFiles int32
		var doneFiles int32

		data, haveVersionData := mojang.FetchVersionData(version, mcDir)
		if !haveVersionData {
			if cached, ok := mojang.CachedVersionData(mcDir, version); ok {
				data = cached
				haveVersionData = true
			}
		}

		if !haveVersionData {
			ch <- DownloadDoneMsg{Err: fmt.Errorf("version metadata unavailable for %s (connect to Mojang or restore the cache)", version)}
			return
		}

		// Contar archivos a descargar
		clientPath := mojang.ClientJarPath(mcDir, version)
		needClient := !mojang.ClientJarExists(mcDir, version)
		if needClient {
			totalFiles++
		}
		librariesPath := filepath.Join(mcDir, "libraries")
		type libJob struct {
			url, path, sha1, name string
		}
		var libJobs []libJob
		if haveVersionData {
			for _, lib := range data.Libraries {
				if !mojang.ShouldIncludeLibrary(lib.Rules) {
					continue
				}
				u := lib.Downloads.Artifact.URL
				p := lib.Downloads.Artifact.Path
				if u != "" && p != "" {
					fullPath := filepath.Join(librariesPath, p)
					libJobs = append(libJobs, libJob{
						url:  u,
						path: fullPath,
						sha1: lib.Downloads.Artifact.SHA1,
						name: lib.Name,
					})
					totalFiles++
				}
			}
		}

		// Fabric
		var fabricJobs []libJob
		if modloader == "Fabric" && !fabric.ProfileExists(mcDir, version) {
			// Estimamos ~5 archivos de Fabric (no los contamos exacto aquí)
			totalFiles += 5
		}

		total := int(totalFiles)
		if total == 0 {
			total = 1
		}

		// Función de descarga con reporte de progreso
		var wg sync.WaitGroup
		downloadFn := mojang.Download(&wg)
		var firstErr error
		var errMu sync.Mutex
		wrappedDownload := func(url, dest, sha1, label string) error {
			err := downloadFn(url, dest, sha1)
			if err != nil {
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
			done := int(atomic.AddInt32(&doneFiles, 1))
			ch <- DownloadProgressMsg{Done: done, Total: total, File: label, Err: err}
			return err
		}

		// 1. client.jar
		if needClient {
			wg.Add(1)
			if err := wrappedDownload(data.Downloads.Client.URL, clientPath,
				data.Downloads.Client.SHA1, "client.jar"); err != nil {
				ch <- DownloadDoneMsg{Err: err}
				return
			}
		}

		// 2. Librerías (paralelo, cap 20)
		if len(libJobs) > 0 {
			sem := make(chan struct{}, 20)
			for _, job := range libJobs {
				j := job
				wg.Add(1)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()
					shortName := filepath.Base(j.path)
					wrappedDownload(j.url, j.path, j.sha1, shortName)
				}()
			}
			wg.Wait()
			if firstErr != nil {
				ch <- DownloadDoneMsg{Err: firstErr}
				return
			}
		}

		// 3. Fabric si aplica
		if modloader == "Fabric" && !fabric.ProfileExists(mcDir, version) {
			ch <- DownloadProgressMsg{
				Done:  int(atomic.LoadInt32(&doneFiles)),
				Total: total,
				File:  "fabric-loader...",
			}
			var fwg sync.WaitGroup
			fDl := mojang.Download(&fwg)
			if _, ok := fabric.Resolve(version, mcDir, filepath.Join(mcDir, "libraries"), &fwg, fDl, 20); !ok {
				ch <- DownloadDoneMsg{Err: fmt.Errorf("Fabric profile unavailable for %s", version)}
				return
			}
			atomic.AddInt32(&doneFiles, 5)
			ch <- DownloadProgressMsg{
				Done:  int(atomic.LoadInt32(&doneFiles)),
				Total: total,
				File:  "fabric-loader (done)",
			}
		}

		_ = fabricJobs // no usado directamente
		ch <- DownloadDoneMsg{}
	}()

	return ch
}
