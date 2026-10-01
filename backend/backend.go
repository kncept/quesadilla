package backend

import (
	"github.com/kncept/quesadilla/backend/definitions"
	"github.com/kncept/quesadilla/backend/llama"
)

// stub, will expand to include more backends
func Backends() []definitions.Backend {
	return []definitions.Backend{
		llama.LlamaBackend(),
	}
}

func Backend(id string) definitions.Backend {
	for _, b := range Backends() {
		if b.Id() == id {
			return b
		}
	}
	return nil
}
