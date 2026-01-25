package user

import (
	"strings"
)

var (
	isUserPubKey = func(input string) bool {
		return strings.HasPrefix(input, "U")
	}
)
