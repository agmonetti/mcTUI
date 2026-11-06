package ui

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"mcTUI/internal/auth"
	"mcTUI/internal/i18n"
)

type deviceCodeInitMsg struct {
	resp   *auth.DeviceCodeResponse
	cancel context.CancelFunc
	err    error
}

type deviceCodeTickMsg struct{}

type deviceCodeAuthResultMsg struct {
	account *auth.Account
	err     error
}

func startDeviceCodeCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		resp, err := auth.RequestDeviceCode(ctx, "")
		return deviceCodeInitMsg{
			resp:   resp,
			cancel: cancel,
			err:    err,
		}
	}
}

func pollDeviceCodeCmd(ctx context.Context, deviceCode string, interval int) tea.Cmd {
	return func() tea.Msg {
		tokens, err := auth.PollDeviceToken(ctx, "", deviceCode, interval, nil)
		if err != nil {
			return deviceCodeAuthResultMsg{err: err}
		}
		acc, err := auth.CompleteDeviceFlow(ctx, tokens)
		return deviceCodeAuthResultMsg{account: acc, err: err}
	}
}

func deviceCodeTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return deviceCodeTickMsg{}
	})
}

func copyToClipboard(text string) error {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("powershell.exe", "-NoProfile", "-Command", "Set-Clipboard", "-Value", text)
		return cmd.Run()
	case "darwin":
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command("wl-copy")
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command("xclip", "-selection", "clipboard")
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			cmd := exec.Command("xsel", "--clipboard", "--input")
			cmd.Stdin = strings.NewReader(text)
			return cmd.Run()
		}
		return fmt.Errorf("no clipboard utility found")
	}
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func (m Model) renderAccountsContent() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("accounts.title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 48))
	b.WriteString(title + "\n" + div + "\n\n")

	if m.authStorage == nil || len(m.authStorage.Accounts) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render(i18n.T("accounts.empty")) + "\n\n")
	} else {
		for i, acc := range m.authStorage.Accounts {
			isActive := acc.ID == m.authStorage.ActiveAccountID
			isCursor := i == m.accountsCursor

			prefix := "  "
			if isCursor {
				prefix = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("► ")
			}

			indicator := "○"
			if isActive {
				indicator = "●"
			}

			tagStyle := lipgloss.NewStyle().Foreground(colorGray)
			tagText := "[Offline]"
			if acc.Type == auth.AccountTypeMicrosoft {
				tagStyle = lipgloss.NewStyle().Foreground(colorGreen)
				tagText = "[Microsoft]"
			}

			userStyle := lipgloss.NewStyle().Foreground(colorWhite)
			if isActive {
				userStyle = userStyle.Bold(true)
			}
			userStr := userStyle.Render(acc.Username)

			activeBadge := ""
			if isActive {
				activeBadge = lipgloss.NewStyle().Foreground(colorAccent2).Render(" (" + i18n.T("accounts.active") + ")")
			}

			line := fmt.Sprintf("%s%s %-16s %s%s", prefix, indicator, userStr, tagStyle.Render(tagText), activeBadge)
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	actionsDiv := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 48))
	b.WriteString(actionsDiv + "\n")

	aAddMsa := lipgloss.NewStyle().Foreground(colorAccent).Render("[A] " + i18n.T("accounts.add_msa"))
	aAddOff := lipgloss.NewStyle().Foreground(colorWhite).Render("[O] " + i18n.T("accounts.add_offline"))
	aDel := lipgloss.NewStyle().Foreground(colorRed).Render("[D] " + i18n.T("accounts.delete"))
	help := lipgloss.NewStyle().Foreground(colorGray).Render("[Enter] " + i18n.T("accounts.switch") + "   [Esc] " + i18n.T("msa.cancel"))

	b.WriteString(aAddMsa + "\n")
	b.WriteString(aAddOff + "\n")
	b.WriteString(aDel + "\n\n")
	b.WriteString(help + "\n")

	return b.String()
}

func (m Model) renderDeviceCodeContent() string {
	var b strings.Builder
	title := lipgloss.NewStyle().Foreground(colorBlue).Bold(true).Render(i18n.T("msa.title"))
	div := lipgloss.NewStyle().Foreground(colorDimGray).Render(strings.Repeat("─", 54))
	b.WriteString(title + "\n" + div + "\n\n")

	if m.deviceCodeErr != nil {
		errStyle := lipgloss.NewStyle().Foreground(colorRed).Bold(true)
		b.WriteString(errStyle.Render(fmt.Sprintf(i18n.T("msa.error"), m.deviceCodeErr.Error())) + "\n\n")
		return b.String()
	}

	if m.deviceCodeResp == nil {
		spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinChar := spinner[m.deviceCodeTick%len(spinner)]
		b.WriteString(lipgloss.NewStyle().Foreground(colorAccent).Render(spinChar + " Obteniendo código de autorización...") + "\n\n")
		return b.String()
	}

	b.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(i18n.T("msa.step1")) + "\n")
	urlLink := lipgloss.NewStyle().Foreground(colorAccent2).Underline(true).Render(m.deviceCodeResp.VerificationURI)
	b.WriteString("  " + urlLink + "\n\n")

	b.WriteString(lipgloss.NewStyle().Foreground(colorWhite).Render(i18n.T("msa.step2")) + "\n\n")

	codeBox := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(colorAccent).
		Foreground(colorAccent2).
		Bold(true).
		Padding(0, 2).
		Render("  " + m.deviceCodeResp.UserCode + "  ")

	b.WriteString("  " + codeBox + "\n\n")

	if m.deviceCodeCopied {
		b.WriteString(lipgloss.NewStyle().Foreground(colorGreen).Bold(true).Render("  ✓ "+i18n.T("msa.copied")) + "\n\n")
	} else {
		b.WriteString("\n")
	}

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinChar := spinner[m.deviceCodeTick%len(spinner)]
	mins := m.deviceCodeExpiresIn / 60
	secs := m.deviceCodeExpiresIn % 60
	waitStr := fmt.Sprintf(i18n.T("msa.waiting"), mins, secs)
	b.WriteString(lipgloss.NewStyle().Foreground(colorGray).Render("  "+spinChar+" "+waitStr) + "\n\n")

	return b.String()
}
