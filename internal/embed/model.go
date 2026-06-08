package embed

import (
	embedfs "embed"

	model2vec "github.com/townsendmerino/aikit/embed"

	"github.com/orafaelfragoso/columbus/internal/contract"
)

//go:embed assets/config.json assets/model.safetensors assets/tokenizer.json
var modelFS embedfs.FS

func loadModel() (*model2vec.StaticModel, error) {
	model, err := model2vec.LoadFromFS(modelFS, "assets")
	if err != nil {
		return nil, embedFailure("load model: %v", err)
	}
	return model, nil
}

func embedFailure(format string, args ...any) error {
	return contract.Errorf(contract.CodeEmbedFailure, format, args...)
}
