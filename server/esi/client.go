package esi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mgibula/eve-industry/server/config"
	"github.com/mgibula/eve-industry/server/db"
	"gorm.io/gorm"
)

type ESIClient struct {
	user db.ESIUser
	db   *gorm.DB
}

type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    uint32 `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

func NewESIClient(db *gorm.DB, user db.ESIUser) ESIClient {
	result := ESIClient{
		user: user,
		db:   db,
	}

	return result
}

func (c *ESIClient) ListSkills() {
	requestParams := url.Values{}
	requestParams.Add("character_id", fmt.Sprint(c.user.ID))
	requestParams.Add("datasource", "tranquility")

	c.makeRequest(http.MethodGet, fmt.Sprintf("/latest/characters/%d/assets/", c.user.ID), requestParams)
}

func (c *ESIClient) GetCharacterInfo() {

}

func (c *ESIClient) makeRequest(method string, uri string, params url.Values) (string, error) {
	apiUrl := "https://esi.evetech.net" + uri
	var requestBody io.Reader

	if method == http.MethodGet {
		apiUrl += "?" + params.Encode()
		requestBody = nil
	} else {
		requestBody = strings.NewReader(params.Encode())
	}

	request, err := http.NewRequest(method, apiUrl, requestBody)
	if err != nil {
		return "", err
	}

	if c.user.ValidUntil.Before(time.Now()) {
		log.Println("Refreshing token", c.user.ValidUntil.String(), time.Now().String())
		newToken, err := c.RefreshToken()
		if err != nil {
			return "", err
		}

		c.user.AccessToken = newToken.AccessToken
		c.user.RefreshToken = newToken.RefreshToken
		c.user.ValidUntil = time.Now().Add(time.Second * time.Duration(newToken.ExpiresIn))
		c.db.Save(&c.user)
	}

	request.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.user.AccessToken))
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	log.Println(response)
	defer response.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func (c *ESIClient) RefreshToken() (*RefreshResponse, error) {
	// Prepare token request
	requestParams := url.Values{}
	requestParams.Add("grant_type", "refresh_token")
	requestParams.Add("refresh_token", c.user.RefreshToken)

	tokenRequest, err := http.NewRequest("POST", "https://login.eveonline.com/v2/oauth/token", strings.NewReader(requestParams.Encode()))
	if err != nil {
		return nil, err
	}

	tokenRequest.Header.Add("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", *config.ClientId, *config.SecretKey)))))
	tokenRequest.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	tokenRequest.Method = http.MethodPost

	// Make a request
	client := &http.Client{}
	tokenResponse, err := client.Do(tokenRequest)
	if err != nil {
		return nil, err
	}

	defer tokenResponse.Body.Close()

	// Read response body
	responseBody, err := ioutil.ReadAll(tokenResponse.Body)
	if err != nil {
		return nil, err
	}

	// Parse response
	responseData := RefreshResponse{}
	err = json.Unmarshal(responseBody, &responseData)
	if err != nil {
		return nil, err
	}

	return &responseData, nil
}
