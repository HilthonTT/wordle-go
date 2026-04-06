package router

import (
	"fmt"
	"strings"
)

func logPath(method, path string) {
	if strings.TrimSpace(path) != "" {
		fmt.Printf("Method: %s - Path: %s\n", method, path)
	}
}
