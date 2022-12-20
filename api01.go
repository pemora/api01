package api01

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	env "github.com/Netflix/go-env"
)

// Wraps the json returned by "/api/auth/token?token=" route
type Token struct {
	Sub          string                 `json:"sub"`
	Iat          int64                  `json:"iat"`
	Exp          int64                  `json:"exp"`
	IP           string                 `json:"ip"`
	Campuses     map[string]interface{} `json:"x-hasura-campuses"`
	HasuraClaims map[string]interface{} `json:"https://hasura.io/jwt/claims"`
}

// Parse & unmarshal raw bytes into a token
func parseToken(rawToken []byte) (Token, error) {
	var token Token

	stripped := strings.Split(string(rawToken), ".")
	rawStrippedToken, err := base64.StdEncoding.DecodeString(stripped[1])
	err = json.Unmarshal([]byte(rawStrippedToken), &token)
	return token, err
}

//The client object used to requests to graphql
type Client struct {
	sync.Mutex
	Endpoint   string
	GiteaToken string `env:"API01_GITEA_TOKEN"`
	RawToken   []byte
	Token      Token
}

// Fetches a token from intra and returns a client object
func NewClient(campusEndpoint string) (Client, error) {
	c := Client{}
	c.Endpoint = campusEndpoint
	_, err := env.UnmarshalFromEnviron(&c)
	if err != nil {
		return c, err
	}

	resp, err := http.Get(fmt.Sprintf("https://%s/api/auth/token?token=%s", campusEndpoint, c.GiteaToken))
	if err != nil {
		return c, err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return c, err
	}

	// Strip double quotes and save raw token for requests
	body = body[1 : len(body)-1]
	c.RawToken = body

	token, err := parseToken(body)
	c.Token = token
	return c, err
}

// Run a request on intra graphql API
func (c *Client) GraphqlQuery(query string) (map[string]interface{}, error) {
	c.Lock()
	defer c.Unlock()
	var responseData map[string]interface{}

	data := map[string]interface{}{
		"query": query,
	}

	body, err := json.Marshal(data)
	req, err := http.NewRequest("POST",
		"https://"+c.Endpoint+"/api/graphql-engine/v1/graphql",
		bytes.NewBuffer(body))
	if err != nil {
		return responseData, err
	}

	headers := map[string]interface{}{
		"Authorization":  "Bearer " + string(c.RawToken),
		"Content-Type":   "application/json",
		"Content-Length": strconv.Itoa(len(query)),
	}

	for k, v := range headers {
		req.Header.Set(k, v.(string))
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return responseData, err
	} else if resp.StatusCode != http.StatusOK {
		return responseData, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return responseData, err
	}

	err = json.Unmarshal(bodyBytes, &responseData)
	if err != nil {
		return responseData, err
	}

	return responseData, nil
}

// Sleep until token is expired and fetches a new one, rince, repeat ;)
//TODO: test !
func (c *Client) RefreshLoop() error {
	for true {
		now := time.Now()
		sec := now.Unix()
		time.Sleep(time.Second * time.Duration(c.Token.Exp-sec))
		err := c.Refresh()
		if err != nil {
			return err
		}
	}
	return nil
}

// Fetch a new token from intra
//TODO: test too!
func (c *Client) Refresh() error {
	c.Lock()
	defer c.Unlock()
	body, err := json.Marshal(map[string]interface{}{"x-jwt-token": c.Token})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST",
		"https://"+c.Endpoint+"/api/auth/refresh",
		bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	body = body[1 : len(body)-1]
	c.RawToken = body
	c.Token, err = parseToken(body)
	return err
}
