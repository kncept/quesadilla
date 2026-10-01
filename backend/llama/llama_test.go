package llama

import (
	"fmt"
	"testing"
)

func TestDetectsFilesForRename(t *testing.T) {
	for _, filename := range llamaFileThatNeedSymlinking() {
		if !shouldSymlinkFile(filename) {
			t.Errorf("Should have needed symlinking: %s", filename)
		}
	}
}

func TestRenameFiles(t *testing.T) {
	fmt.Printf("\n")
	for _, filename := range llamaFileThatNeedSymlinking() {
		newname := newFileName(filename)
		fmt.Printf("%s  ->. %s\n", filename, newname)
	}
	fmt.Printf("\n")
}

func llamaFileThatNeedSymlinking() []string {
	return []string{
		"libggml-base.0.25.3.dylib",
		"libggml-blas.0.25.3.dylib",
		"libggml-cpu.0.25.3.dylib",
		"libggml-metal.0.25.3.dylib",
		"libggml-rpc.0.25.3.dylib",
		"libggml.0.25.3.dylib",
		"libllama-common.0.5.0.dylib",
		"libllama.0.5.0.dylib",
		"libmtmd.0.5.0.dylib",
	}
}
