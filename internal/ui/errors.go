package ui

import (
	"strings"

	"mcTUI/internal/i18n"
)

var appErrorKeys = map[string]string{
	"cannot rename this instance":                                  "error.cannot_rename_instance",
	"invalid instance settings":                                    "error.invalid_instance_settings",
	"Minecraft version and loader are fixed for managed instances": "error.managed_versions_fixed",
	"file changed":                                    "error.file_changed",
	"file changed while reading":                      "error.file_changed_reading",
	"select a local .jar file":                        "error.select_local_jar",
	"JAR too large":                                   "error.jar_too_large",
	"source changed":                                  "error.source_changed",
	"explicit removal confirmation required":          "error.removal_confirmation_required",
	"unsafe filename":                                 "error.unsafe_filename",
	"external change detected; file preserved":        "error.external_change_preserved",
	"managed file changed externally; file preserved": "error.managed_change_preserved",
}

var appErrorPrefixes = map[string]string{
	"destination collision: ":                 "error.destination_collision",
	"retired instance already exists: ":       "error.retired_instance_exists",
	"instance already exists: ":               "error.instance_exists",
	"could not remove retired instance ":      "error.remove_retired_instance",
	"instance removed but backups remain at ": "error.backups_remain",
}

// displayError translates mcTUI-owned error text and leaves all other details intact.
func displayError(err error) string {
	if err == nil {
		return ""
	}
	text := err.Error()
	if path, detail, ok := splitError(text, "file added at ", " but registry failed; refresh: "); ok {
		return i18n.T("error.registry_failed_add", path, detail)
	}
	if path, detail, ok := splitError(text, "file operation completed in ", " but registry failed; refresh: "); ok {
		return i18n.T("error.registry_failed_operation", path, detail)
	}
	if path, detail, ok := splitError(text, "copy incomplete; partial data retained at ", ": "); ok {
		return i18n.T("error.copy_incomplete", path, detail)
	}
	if path, detail, ok := splitError(text, "instance busy (", "): "); ok {
		return i18n.T("error.instance_busy", path, detail)
	}
	if key, ok := appErrorKeys[text]; ok {
		return i18n.T(key)
	}
	for prefix, key := range appErrorPrefixes {
		if strings.HasPrefix(text, prefix) {
			return i18n.T(key, strings.TrimPrefix(text, prefix))
		}
	}
	return text
}

func splitError(text, prefix, separator string) (string, string, bool) {
	if !strings.HasPrefix(text, prefix) {
		return "", "", false
	}
	path, detail, ok := strings.Cut(strings.TrimPrefix(text, prefix), separator)
	return path, detail, ok
}
