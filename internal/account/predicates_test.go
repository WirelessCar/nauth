package account

import (
	"strings"
)

var (
	isAccountPubKey = func(input string) bool {
		return strings.HasPrefix(input, "A")
	}
	isUserPubKey = func(input string) bool {
		return strings.HasPrefix(input, "U")
	}
)
