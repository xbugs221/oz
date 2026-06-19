// Package app contains small generic helpers shared across workflow views.
package app

// nonEmpty returns value unless it is empty, otherwise fallback.
func nonEmpty(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
