package apiv1

import (
	"time"

	"github.com/partyoffice/spotifete/database/model"
)

type ErrorResponse struct {
	Message string `json:"message"`
}

type GetAuthUrlResponse struct {
	Url       string `json:"url"`
	SessionId string `json:"sessionId"`
}

type DidAuthSucceedResponse struct {
	Authenticated bool `json:"authenticated"`
}

type SearchTracksResponse struct {
	Query   string                `json:"query"`
	Results []model.TrackMetadata `json:"results"`
}

type SearchPlaylistResponse struct {
	Query   string                   `json:"query"`
	Results []model.PlaylistMetadata `json:"results"`
}

type QueueLastUpdatedResponse struct {
	QueueLastUpdated time.Time `json:"queueLastUpdated"`
}
