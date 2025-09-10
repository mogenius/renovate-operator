package assert

import (
	"fmt"
	"os"
)

func NoError(err error, message string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", message)
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
}
