package agent

import (
	"context"
	"crypto/sha256"
	"fmt"

	"troy/model"
	"troy/sandbox"
)

func hydrateDocuments(ctx context.Context, sb sandbox.Sandbox, messages []model.Message) ([]model.Message, error) {
	out := append([]model.Message{}, messages...)
	loaded := map[string][]byte{}
	for i, message := range messages {
		if len(message.Documents) == 0 {
			continue
		}
		out[i].Documents = append([]model.Document{}, message.Documents...)
		for j, doc := range message.Documents {
			key := doc.Path + ":" + doc.SHA256
			data, ok := loaded[key]
			if !ok {
				var err error
				data, err = sb.ReadFile(ctx, doc.Path)
				if err != nil {
					return nil, fmt.Errorf("read attached PDF %s: %w", doc.Name, err)
				}
				if fmt.Sprintf("%x", sha256.Sum256(data)) != doc.SHA256 {
					return nil, fmt.Errorf("attached PDF %s changed; reload the original with read_documents", doc.Name)
				}
				loaded[key] = data
			}
			out[i].Documents[j].Data = data
		}
	}
	return out, nil
}
