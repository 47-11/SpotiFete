package controller

import (
	"fmt"
	"github.com/47-11/spotifete/authentication"
	"github.com/47-11/spotifete/config"
	"github.com/47-11/spotifete/database/model"
	"github.com/47-11/spotifete/listeningSession"
	"github.com/47-11/spotifete/users"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"time"
)

type TemplateController struct{ Controller }

func (c TemplateController) SetupWithBaseRouter(baseRouter *gin.Engine) {
	baseRouter.LoadHTMLGlob("resources/templates/*.html")

	baseRouter.GET("/", c.Index)
	baseRouter.GET("/login", c.Login)
	baseRouter.GET("/logout", c.Logout)
	baseRouter.GET("/session/new", c.NewListeningSession)
	baseRouter.POST("/session/new", c.NewListeningSessionSubmit)
	baseRouter.GET("/session/view/:joinId", c.ViewSession)
	baseRouter.POST("/session/view/:joinId/request", c.RequestTrack)
	baseRouter.POST("/session/view/:joinId/fallback", c.ChangeFallbackPlaylist)
	baseRouter.POST("/session/close", c.CloseListeningSession)
	baseRouter.GET("/app", c.GetApp)
	baseRouter.GET("/app/android", c.GetAppAndroid)
	baseRouter.GET("/app/ios", c.GetAppIOS)
}

func (TemplateController) Index(c *gin.Context) {
	loginSession := authentication.GetValidSessionFromCookie(c)
	if loginSession == nil || loginSession.UserId == nil {
		c.HTML(http.StatusOK, "index.html", gin.H{
			"time":               time.Now(),
			"activeSessionCount": listeningSession.GetActiveSessionCount(),
			"totalSessionCount":  listeningSession.GetTotalSessionCount(),
			"user":               nil,
			"userSessions":       nil,
		})
		return
	}

	// TODO: Use eager loading
	loggedInUser := users.FindFullUser(model.SimpleUser{
		Model: gorm.Model{ID: *loginSession.UserId},
	})
	c.HTML(http.StatusOK, "index.html", gin.H{
		"time":               time.Now(),
		"activeSessionCount": listeningSession.GetActiveSessionCount(),
		"totalSessionCount":  listeningSession.GetTotalSessionCount(),
		"user":               loggedInUser,
		"userSessions":       loggedInUser.ListeningSessions,
	})
}

func (TemplateController) Login(c *gin.Context) {
	redirectTo := c.DefaultQuery("redirectTo", "/")

	_, authUrl := authentication.NewSession(redirectTo)
	c.Redirect(http.StatusTemporaryRedirect, authUrl)
}

func (TemplateController) Logout(c *gin.Context) {
	sessionId := authentication.GetSessionIdFromCookie(c)
	if sessionId != nil {
		authentication.InvalidateSession(*sessionId)
		authentication.RemoveCookie(c)
	}

	redirectTo := c.DefaultQuery("redirectTo", "/")
	if redirectTo[0:1] != "/" {
		redirectTo = "/" + redirectTo
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectTo)
}

func (TemplateController) NewListeningSession(c *gin.Context) {
	loginSession := authentication.GetValidSessionFromCookie(c)
	if loginSession == nil || loginSession.UserId == nil {
		c.Redirect(http.StatusSeeOther, "/login?redirectTo=/session/new")
		return
	}

	// TODO: Use eager loading
	loggedInUser := users.FindSimpleUser(model.SimpleUser{
		Model: gorm.Model{ID: *loginSession.UserId},
	})
	c.HTML(http.StatusOK, "newSession.html", gin.H{
		"user": loggedInUser,
	})
}

func (TemplateController) NewListeningSessionSubmit(c *gin.Context) {
	loginSession := authentication.GetValidSessionFromCookie(c)
	if loginSession == nil || loginSession.UserId == nil {
		c.Redirect(http.StatusSeeOther, "/login?redirectTo=/session/new")
		return
	}

	// TODO: Use eager loading
	loggedInUser := users.FindSimpleUser(model.SimpleUser{
		Model: gorm.Model{ID: *loginSession.UserId},
	})

	title := c.PostForm("title")
	if len(title) == 0 {
		c.String(http.StatusBadRequest, "Title must not be empty.")
		return
	}

	session, spotifeteError := listeningSession.NewSession(*loggedInUser, title)
	if spotifeteError != nil {
		spotifeteError.SetStringResponse(c)
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/session/view/%s", *session.JoinId))
}

func (TemplateController) ViewSession(c *gin.Context) {
	joinId := c.Param("joinId")
	session := listeningSession.GetSessionByJoinId(joinId)
	if session == nil {
		c.String(http.StatusNotFound, "Session not found.")
		return
	}

	ListeningSessionDto := listeningSession.CreateDto(*session, true)

	displayError := c.Query("displayError")

	queueLastUpdated := listeningSession.GetQueueLastUpdated(*session).UTC().Format(time.RFC3339Nano)
	loginSession := authentication.GetValidSessionFromCookie(c)
	if loginSession == nil || loginSession.UserId == nil {
		c.HTML(http.StatusOK, "viewSession.html", gin.H{
			"queueLastUpdated": queueLastUpdated,
			"session":          ListeningSessionDto,
			"displayError":     displayError,
		})
		return
	}

	// TODO: Use eager loading
	loggedInUser := users.FindSimpleUser(model.SimpleUser{
		Model: gorm.Model{ID: *loginSession.UserId},
	})
	c.HTML(http.StatusOK, "viewSession.html", gin.H{
		"queueLastUpdated": queueLastUpdated,
		"session":          ListeningSessionDto,
		"user":             loggedInUser,
		"displayError":     displayError,
	})
}

func (TemplateController) RequestTrack(c *gin.Context) {
	joinId := c.Param("joinId")
	session := listeningSession.GetSessionByJoinId(joinId)
	if session == nil {
		c.String(http.StatusNotFound, "session not found")
		return
	}

	trackId := c.PostForm("trackId")

	_, spotifeteError := listeningSession.RequestSong(*session, trackId)
	if spotifeteError == nil {
		c.Redirect(http.StatusSeeOther, "/session/view/"+joinId)
	} else {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/session/view/%s/?displayError=%s", joinId, spotifeteError.MessageForUser))
	}
}

func (TemplateController) ChangeFallbackPlaylist(c *gin.Context) {
	joinId := c.Param("joinId")
	session := listeningSession.GetSessionByJoinId(joinId)
	if session == nil {
		c.String(http.StatusNotFound, "session not found")
		return
	}

	loginSession := authentication.GetValidSessionFromCookie(c)
	if loginSession == nil || loginSession.UserId == nil {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/login?redirectTo=/session/view/%s", joinId))
		return
	}

	// TODO: Use eager loading
	loggedInUser := users.FindSimpleUser(model.SimpleUser{
		Model: gorm.Model{ID: *loginSession.UserId},
	})

	playlistId := c.PostForm("playlistId")
	spotifeteError := listeningSession.ChangeFallbackPlaylist(*session, *loggedInUser, playlistId)
	if spotifeteError == nil {
		c.Redirect(http.StatusSeeOther, "/session/view/"+joinId)
	} else {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/session/view/%s/?displayError=%s", joinId, spotifeteError.MessageForUser))
	}
}

func (TemplateController) CloseListeningSession(c *gin.Context) {
	joinId := c.PostForm("joinId")
	if len(joinId) == 0 {
		c.String(http.StatusBadRequest, "parameter joinId not present")
		return
	}

	loginSession := authentication.GetValidSessionFromCookie(c)
	if loginSession == nil || loginSession.UserId == nil {
		c.Redirect(http.StatusUnauthorized, fmt.Sprintf("/login?redirectTo=/session/view/%s", joinId))
		return
	}

	// TODO: Use eager loading
	loggedInUser := users.FindSimpleUser(model.SimpleUser{
		Model: gorm.Model{ID: *loginSession.UserId},
	})

	spotifeteError := listeningSession.CloseSession(*loggedInUser, joinId)
	if spotifeteError != nil {
		spotifeteError.SetStringResponse(c)
		return
	}

	c.Redirect(http.StatusSeeOther, "/")
}

func (TemplateController) GetApp(c *gin.Context) {
	c.Redirect(http.StatusTemporaryRedirect, "/app/android")
}

func (TemplateController) GetAppAndroid(c *gin.Context) {
	androidUrl := config.Get().SpotifeteConfiguration.AppConfiguration.AndroidUrl
	if androidUrl == nil {
		c.String(http.StatusNotImplemented, "Sorry, the android app is not available!")
	} else {
		c.Redirect(http.StatusTemporaryRedirect, *androidUrl)
	}
}

func (TemplateController) GetAppIOS(c *gin.Context) {
	iosUrl := config.Get().SpotifeteConfiguration.AppConfiguration.IOsUrl
	if iosUrl == nil {
		c.String(http.StatusNotImplemented, "Sorry, the iOS app is not available!")
	} else {
		c.Redirect(http.StatusTemporaryRedirect, *iosUrl)
	}
}
