package esi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
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

type esiResponse struct {
	status     int
	body       string
	error      error
	etag       string
	validUntil time.Time
	is_valid   bool
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
	requestParams.Add("datasource", "tranquility")

	response := c.makeRequest(http.MethodGet, fmt.Sprintf("/latest/characters/%d/skills/", c.user.ID), requestParams)
	log.Println(response)
}

func (c *ESIClient) GetCharacterInfo() {

}

func (c *ESIClient) fetchFromCache(method string, url string, params string) *esiResponse {
	var cached db.ESICall

	err := c.db.Where("url = ? and params = ?", url, params).Order("valid_until desc").First(&cached).Error
	if err != nil {
		log.Println(url, "not in cache")
		return nil
	}

	is_valid := cached.ValidUntil.After(time.Now())
	if !is_valid && cached.Etag == "" {
		log.Println(url, "has expired and no etag")
		c.db.Delete(&db.ESICall{}, "url = ? and params = ?", url, params)
		return nil
	}

	return &esiResponse{
		status:     200,
		body:       cached.Response,
		validUntil: cached.ValidUntil,
		etag:       cached.Etag,
		is_valid:   is_valid,
	}
}

func (c *ESIClient) saveToCache(method string, url string, params string, response esiResponse) {
	c.db.Create(&db.ESICall{
		URL:        url,
		Params:     params,
		Response:   response.body,
		ValidUntil: response.validUntil,
		Etag:       response.etag,
	})
}

func (c *ESIClient) makeRequest(method string, uri string, params url.Values) esiResponse {
	paramsCacheKey := params.Encode()
	maybe_cached := c.fetchFromCache(method, uri, paramsCacheKey)
	if maybe_cached != nil && maybe_cached.is_valid {
		return *maybe_cached
	}

	apiUrl := "https://esi.evetech.net" + uri
	var requestBody io.Reader

	if maybe_cached != nil && maybe_cached.etag != "" {
		params.Add("If-None-Match", maybe_cached.etag)
	}

	if method == http.MethodGet {
		apiUrl += "?" + params.Encode()
		requestBody = nil
	} else {
		requestBody = strings.NewReader(params.Encode())
	}

	request, err := http.NewRequest(method, apiUrl, requestBody)
	if err != nil {
		return esiResponse{
			error: err,
		}
	}

	if c.user.ValidUntil.Before(time.Now()) {
		log.Println("Refreshing token", c.user.ValidUntil.String(), time.Now().String())
		newToken, err := c.RefreshToken()
		if err != nil {
			return esiResponse{
				error: err,
			}
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
		return esiResponse{
			error: err,
		}
	}

	defer response.Body.Close()

	// Read response body
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return esiResponse{
			error: err,
		}
	}

	expires, err := time.Parse(time.RFC1123, response.Header.Get("Expires"))
	if err != nil {
		log.Println("Expires header invalid format", response.Header.Get("Expires"), err)
		expires = time.Now()
	}

	var result esiResponse
	if response.StatusCode >= 400 {
		result.error = errors.New(string(body))
	} else if response.StatusCode == 304 && maybe_cached != nil {
		log.Println("304 response code, using cached version")
		result.body = maybe_cached.body
	} else {
		result.body = string(body)
	}

	result.status = response.StatusCode
	result.etag = response.Header.Get("ETag")
	result.validUntil = expires

	c.saveToCache(method, uri, paramsCacheKey, result)

	return result
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
