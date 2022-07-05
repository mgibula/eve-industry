package sso

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/mgibula/eve-industry/server/config"
	"github.com/mgibula/eve-industry/server/sessions"
)

func ssoRedirectHandler(c *gin.Context) {
	query := url.URL{
		Scheme: "https",
		Host:   "login.eveonline.com",
		Path:   "/v2/oauth/authorize/",
	}

	permissions := []string{
		"esi-assets.read_assets.v1",
		"esi-industry.read_character_jobs.v1",
		"esi-characters.read_blueprints.v1",
		"esi-assets.read_corporation_assets.v1",
		"esi-corporations.read_blueprints.v1",
		"esi-industry.read_corporation_jobs.v1",
		"esi-skills.read_skills.v1",
	}

	ssoState := fmt.Sprint(rand.Uint64())
	queryString := query.Query()

	queryString.Add("response_type", "code")
	queryString.Add("redirect_uri", "http://localhost:8080/sso/callback")
	queryString.Add("client_id", *config.ClientId)
	queryString.Add("scope", strings.Join(permissions, " "))
	queryString.Add("state", ssoState)

	query.RawQuery = queryString.Encode()
	session := sessions.OpenSession(c)
	session.Set("sso_state", ssoState)

	c.Redirect(http.StatusFound, query.String())
}
