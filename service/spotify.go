package service

import (
	"github.com/47-11/spotifete/config"
	"github.com/47-11/spotifete/database"
	spotifeteError "github.com/47-11/spotifete/error"
	. "github.com/47-11/spotifete/model/database"
	"github.com/47-11/spotifete/model/dto"
	"github.com/jinzhu/gorm"
	"github.com/zmb3/spotify"
	"strings"
	"sync"
)

type spotifyService struct {
	Authenticator spotify.Authenticator
	Clients       map[string]*spotify.Client
}

var spotifyServiceInstance *spotifyService
var spotifyServiceOnce sync.Once

func SpotifyService() *spotifyService {
	spotifyServiceOnce.Do(func() {
		c := config.Get()
		callbackUrl := c.SpotifeteConfiguration.BaseUrl + "/spotify/callback"

		newAuth := spotify.NewAuthenticator(callbackUrl, spotify.ScopePlaylistReadPrivate, spotify.ScopePlaylistModifyPrivate, spotify.ScopeImageUpload, spotify.ScopeUserLibraryRead, spotify.ScopeUserModifyPlaybackState, spotify.ScopeUserReadCurrentlyPlaying, spotify.ScopeUserReadPrivate)
		newAuth.SetAuthInfo(c.SpotifyConfiguration.Id, c.SpotifyConfiguration.Secret)

		spotifyServiceInstance = &spotifyService{
			Authenticator: newAuth,
			Clients:       map[string]*spotify.Client{},
		}
	})
	return spotifyServiceInstance
}

func (s spotifyService) GetClientForSpotifyUser(spotifyUserId string) *spotify.Client {
	if client, ok := s.Clients[spotifyUserId]; ok {
		s.refreshAndSaveTokenForSpotifyUserIfNeccessary(*client, spotifyUserId)
		return client
	}

	user := UserService().GetUserBySpotifyId(spotifyUserId)
	return s.GetClientForUser(*user)
}

func (s spotifyService) GetClientForUser(user User) *spotify.Client {
	if client, ok := s.Clients[user.SpotifyId]; ok {
		s.refreshAndSaveTokenForUserIfNeccessary(*client, user)
		return client
	}

	token := user.GetToken()
	if token == nil {
		return nil
	}

	client := s.Authenticator.NewClient(token)
	s.refreshAndSaveTokenForUserIfNeccessary(client, user)
	s.Clients[user.SpotifyId] = &client

	return &client
}

func (s spotifyService) refreshAndSaveTokenForSpotifyUserIfNeccessary(client spotify.Client, spotifyUserId string) {
	user := UserService().GetUserBySpotifyId(spotifyUserId)
	s.refreshAndSaveTokenForUserIfNeccessary(client, *user)
}

func (s spotifyService) refreshAndSaveTokenForUserIfNeccessary(client spotify.Client, user User) {
	newToken, err := client.Token() // This should refresh the token if neccessary: https://github.com/zmb3/spotify/issues/108#issuecomment-568899119
	if err != nil {
		spotifeteError.IllegalState{}.WithCause(err).WithMessage("Could not refresh token.")
		return
	}

	if newToken.Expiry.After(user.SpotifyTokenExpiry) {
		// Token was updated, persist to database
		// Do this in a goroutine so API calls don't have to wait for the database write to succeed
		go UserService().SetToken(user, *newToken)
	}
}

func (s spotifyService) NewAuthUrl(callbackRedirectUrl string) (authUrl string, sessionId string) {
	sessionId = LoginSessionService().newSessionId()
	database.GetConnection().Create(&LoginSession{
		Model:            gorm.Model{},
		SessionId:        sessionId,
		UserId:           nil,
		Active:           true,
		CallbackRedirect: callbackRedirectUrl,
	})
	return s.Authenticator.AuthURL(sessionId), sessionId
}

func (s spotifyService) SearchTrack(client spotify.Client, query string, limit int) ([]dto.TrackMetadataDto, error) {
	cleanedQuery := strings.TrimSpace(query) + "*"

	currentUser, err := client.CurrentUser()
	if err != nil {
		return nil, spotifeteError.IllegalState{}.WithCause(err).WithMessage("Could not get information on current user from Spotify.").Build()
	}

	result, err := client.SearchOpt(cleanedQuery, spotify.SearchTypeTrack, &spotify.Options{
		Limit:   &limit,
		Country: &currentUser.Country,
	})
	if err != nil {
		return nil, spotifeteError.IllegalState{}.WithCause(err).WithMessage("Track search failed.").Build()
	}

	var resultDtos []dto.TrackMetadataDto
	for _, track := range result.Tracks.Tracks {
		metadata := TrackMetadata{}.SetMetadata(track)
		resultDtos = append(resultDtos, dto.TrackMetadataDto{}.FromDatabaseModel(metadata))
	}

	return resultDtos, nil
}

func (s spotifyService) SearchPlaylist(client spotify.Client, query string, limit int) ([]dto.PlaylistMetadataDto, error) {
	cleanedQuery := strings.TrimSpace(query) + "*"
	result, err := client.SearchOpt(cleanedQuery, spotify.SearchTypePlaylist, &spotify.Options{
		Limit: &limit,
	})
	if err != nil {
		return nil, spotifeteError.IllegalState{}.WithCause(err).WithMessage("Playlist search failed.").Build()
	}

	var resultDtos []dto.PlaylistMetadataDto
	for _, playlist := range result.Playlists.Playlists {
		resultDtos = append(resultDtos, dto.PlaylistMetadataDto{}.FromDatabaseModel(PlaylistMetadata{}.FromSimplePlaylist(playlist)))
	}

	return resultDtos, nil
}

func (s spotifyService) AddOrUpdateTrackMetadata(client spotify.Client, spotifyTrack spotify.FullTrack) (TrackMetadata, error) {
	track := s.GetTrackMetadataBySpotifyTrackId(spotifyTrack.ID.String())
	if track != nil {
		updatedTrack := track.SetMetadata(spotifyTrack)

		database.GetConnection().Save(&updatedTrack)

		return updatedTrack, nil
	} else {
		newTrack := TrackMetadata{}.SetMetadata(spotifyTrack)

		database.GetConnection().Create(&newTrack)

		return newTrack, nil
	}
}

func (s spotifyService) GetTrackMetadataBySpotifyTrackId(trackId string) *TrackMetadata {
	var foundTracks []TrackMetadata
	database.GetConnection().Where(TrackMetadata{SpotifyTrackId: trackId}).Find(&foundTracks)

	if len(foundTracks) > 0 {
		return &foundTracks[0]
	} else {
		return nil
	}
}

func (s spotifyService) AddOrUpdatePlaylistMetadata(client spotify.Client, playlistId spotify.ID) (PlaylistMetadata, error) {
	spotifyPlaylist, err := client.GetPlaylist(playlistId)
	if err != nil {
		return PlaylistMetadata{}, spotifeteError.IllegalState{}.WithCause(err).WithMessage("Could not get playlist information from Spotify.").Build()
	}

	knownPlaylistMetadata := s.GetPlaylistMetadataBySpotifyPlaylistId(playlistId.String())
	if knownPlaylistMetadata != nil {
		updatedPlaylistMetadata := knownPlaylistMetadata.FromFullPlaylist(*spotifyPlaylist)

		database.GetConnection().Save(&updatedPlaylistMetadata)

		return updatedPlaylistMetadata, nil
	} else {
		newPlaylistMetadata := PlaylistMetadata{}.FromFullPlaylist(*spotifyPlaylist)

		database.GetConnection().Create(&newPlaylistMetadata)

		return newPlaylistMetadata, nil
	}
}

func (s spotifyService) GetPlaylistMetadataBySpotifyPlaylistId(playlistId string) *PlaylistMetadata {
	var foundPlaylists []PlaylistMetadata
	database.GetConnection().Where(PlaylistMetadata{SpotifyPlaylistId: playlistId}).Find(&foundPlaylists)

	if len(foundPlaylists) > 0 {
		return &foundPlaylists[0]
	} else {
		return nil
	}
}
