package sso

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mgibula/eve-industry/server/config"
	"github.com/mgibula/eve-industry/server/db"
	"github.com/mgibula/eve-industry/server/sessions"

	jwks "github.com/MicahParks/keyfunc"
	jwt "github.com/golang-jwt/jwt/v4"
)

type ssoResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    uint32 `json:"expires_in"`
	Type         string `json:"Bearer"`
	RefreshToken string `json:"refresh_token"`
}

type callbackParams struct {
	Code  string `form:"code"`
	State string `form:"state"`
}

var jwksKeys *jwks.JWKS

func init() {
	keys, err := jwks.Get("https://login.eveonline.com/oauth/jwks", jwks.Options{})
	if err != nil {
		log.Fatalf("Failed to get the JWKS from the given URL.\nError:%s", err.Error())
	}

	jwksKeys = keys

	jwt.TimeFunc = func() time.Time {
		return time.Now().UTC().Add(time.Second * 20)
	}
}

func ssoCallbackHandler(c *gin.Context) {
	session := sessions.OpenSession(c)

	// Bind params
	var params callbackParams
	c.BindQuery(&params)

	// Verify SSO state
	ssoState, exists := session.Get("sso_state").(string)
	if exists && ssoState != params.State {
		c.String(http.StatusBadRequest, "SSO state mismach. Please retry")
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	session.Delete("sso_state")

	// Prepare token request
	requestParams := url.Values{}
	requestParams.Add("grant_type", "authorization_code")
	requestParams.Add("code", params.Code)

	tokenRequest, err := http.NewRequest("POST", "https://login.eveonline.com/v2/oauth/token", strings.NewReader(requestParams.Encode()))
	if err != nil {
		c.String(http.StatusInternalServerError, "Error while creating OAuth request", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	tokenRequest.Header.Add("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", *config.ClientId, *config.SecretKey)))))
	tokenRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	tokenRequest.Method = http.MethodPost

	// Make a request
	client := &http.Client{}
	tokenResponse, err := client.Do(tokenRequest)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error while getting OAuth token", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	defer tokenResponse.Body.Close()

	// Read response body
	responseBody, err := ioutil.ReadAll(tokenResponse.Body)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error while reading response body", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Parse response
	responseData := ssoResponse{}
	err = json.Unmarshal(responseBody, &responseData)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error while deserializing response body", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Validate JWT
	token, err := jwt.Parse(responseData.AccessToken, jwksKeys.Keyfunc)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error while parsing JWT token", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		c.String(http.StatusInternalServerError, "Error while validating JWT token")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	if claims["aud"] != "EVE Online" {
		c.String(http.StatusInternalServerError, "Unknown JWT Audience", claims["aud"])
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// Parse JWT
	parts := strings.Split(claims["sub"].(string), ":")
	if parts[0] != "CHARACTER" || parts[1] != "EVE" {
		c.String(http.StatusInternalServerError, "Unknown JWT Subject", claims["sub"])
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	characterId, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		c.String(http.StatusInternalServerError, "Error while parsing character ID", err)
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	characterName := claims["name"].(string)

	expires := time.Now().Add(time.Second * time.Duration(responseData.ExpiresIn))

	manager := db.OpenEveDatabase()
	esiUser := db.ESIUser{
		ID:            characterId,
		CharacterName: characterName,
		RefreshToken:  responseData.RefreshToken,
		AccessToken:   responseData.AccessToken,
		ValidUntil:    expires,
	}

	result := manager.Find(&esiUser)
	if result.RowsAffected > 0 {
		manager.Save(&esiUser)
	} else {
		manager.Create(&esiUser)
	}

	loggedCharacters, exists := session.Get("available_users").([]db.ESIUser)
	if !exists {
		loggedCharacters = make([]db.ESIUser, 0)
	}

	loggedCharacters = append(loggedCharacters, esiUser)
	session.Set("available_users", uniqueUsers(loggedCharacters))
	session.Set("current_user", esiUser)

	c.Redirect(http.StatusFound, "/")
}

func uniqueUsers(users []db.ESIUser) []db.ESIUser {
	result := make([]db.ESIUser, 0, len(users))
	keys := make(map[uint64]bool)

	for _, user := range users {
		if _, value := keys[user.ID]; !value {
			keys[user.ID] = true
			result = append(result, user)
		}
	}

	return result
}
