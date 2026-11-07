package catalog

import "regexp"

var idPattern = regexp.MustCompile(`^[0-7][0-9A-HJKMNP-TV-Z]{25}$`)

func ValidID(id string) bool { return idPattern.MatchString(id) }
