package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"
)

// cosmosContainer is the slice of *azcosmos.ContainerClient the persistence
// layer uses. Defined as an interface so tests can fake it; production wires
// the real ContainerClient.
type cosmosContainer interface {
	ReadItem(ctx context.Context, partitionKey azcosmos.PartitionKey, itemID string, opts *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
	UpsertItem(ctx context.Context, partitionKey azcosmos.PartitionKey, item []byte, opts *azcosmos.ItemOptions) (azcosmos.ItemResponse, error)
}

type cosmosStore struct {
	container cosmosContainer
	docID     string
}

type cosmosDoc struct {
	ID    string              `json:"id"`
	State persistedAtmosphere `json:"state"`
}

func newCosmosStoreFromEnv() (*cosmosStore, error) {
	endpoint := os.Getenv("AMBIENCE_COSMOS_ENDPOINT")
	if endpoint == "" {
		return nil, nil
	}
	database := os.Getenv("AMBIENCE_COSMOS_DATABASE")
	if database == "" {
		return nil, errors.New("AMBIENCE_COSMOS_DATABASE required when AMBIENCE_COSMOS_ENDPOINT is set")
	}
	containerName := os.Getenv("AMBIENCE_COSMOS_CONTAINER")
	if containerName == "" {
		return nil, errors.New("AMBIENCE_COSMOS_CONTAINER required when AMBIENCE_COSMOS_ENDPOINT is set")
	}
	docID := os.Getenv("AMBIENCE_COSMOS_DOCUMENT_ID")
	if docID == "" {
		docID = "shared"
	}

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("create azure credential: %w", err)
	}
	client, err := azcosmos.NewClient(endpoint, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("create cosmos client: %w", err)
	}
	container, err := client.NewContainer(database, containerName)
	if err != nil {
		return nil, fmt.Errorf("create cosmos container client: %w", err)
	}
	return &cosmosStore{container: container, docID: docID}, nil
}

func (s *cosmosStore) Load(ctx context.Context) (*persistedAtmosphere, error) {
	pk := azcosmos.NewPartitionKeyString(s.docID)
	resp, err := s.container.ReadItem(ctx, pk, s.docID, nil)
	if err != nil {
		if isCosmosStatus(err, http.StatusNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cosmos item: %w", err)
	}
	var doc cosmosDoc
	if err := json.Unmarshal(resp.Value, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal cosmos item: %w", err)
	}
	if doc.State.Version != 1 {
		return nil, fmt.Errorf("unsupported persisted state version %d", doc.State.Version)
	}
	return &doc.State, nil
}

func (s *cosmosStore) Save(ctx context.Context, state persistedAtmosphere) error {
	doc := cosmosDoc{ID: s.docID, State: state}
	payload, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal cosmos doc: %w", err)
	}
	pk := azcosmos.NewPartitionKeyString(s.docID)
	if _, err := s.container.UpsertItem(ctx, pk, payload, nil); err != nil {
		return fmt.Errorf("upsert cosmos item: %w", err)
	}
	return nil
}

func isCosmosStatus(err error, status int) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr.StatusCode == status
	}
	return false
}
