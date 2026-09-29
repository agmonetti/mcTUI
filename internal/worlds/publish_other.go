//go:build !linux && !darwin && !windows

package worlds

import "fmt"

func publishWorld(stage, destination string) error {
	return fmt.Errorf("atomic world publication is unsupported on this platform")
}
