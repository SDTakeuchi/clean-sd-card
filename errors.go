package main

import "fmt"

type fileCopyError struct {
	fileName string
	err      error
}

func (e fileCopyError) Error() string {
	return fmt.Sprintf("copying file %s: %s", e.fileName, e.err.Error())
}
