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

type Vars map[string]interface{}

// Wrapper for responses from graphiql
type GraphqlResponse struct {
	Data   map[string]interface{}
	Errors []error
}

func (g *GraphqlResponse) HasErrors() bool {
	if len(g.Errors) > 0 {
		return true
	}
	return false
}

type QueryError struct {
	message string
}

func (qe QueryError) Error() string {
	return qe.message
}

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
	if len(stripped) < 2 {
		return nil,log.Errorf("invalid token")
	}
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

	resp, err := http.Get("https://" + campusEndpoint + "/api/auth/token?token=" + c.GiteaToken)
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

// Run a request on intra graphql API
func (c *Client) GraphqlQuery(query string, variables Vars) GraphqlResponse {
	c.Lock()
	defer c.Unlock()

	var responseData map[string]interface{}
	var g GraphqlResponse

	data := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	body, err := json.Marshal(data)
	req, err := http.NewRequest("POST",
		"https://"+c.Endpoint+"/api/graphql-engine/v1/graphql",
		bytes.NewBuffer(body))
	if err != nil {
		g.Errors = append(g.Errors, err)
		return g
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
		g.Errors = append(g.Errors, err)
		return g
	} else if resp.StatusCode != http.StatusOK {
		g.Errors = append(g.Errors, QueryError{
			fmt.Sprintf("unexpected status code: %d", resp.StatusCode),
		})
		return g

	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		g.Errors = append(g.Errors, err)
		return g
	}

	err = json.Unmarshal(bodyBytes, &responseData)
	if err != nil {
		fmt.Println(err)
		g.Errors = append(g.Errors, err)
		return g
	}

	if errors, _ := responseData["errors"].([]map[string]map[string]string); len(errors) > 0 {
		for _, e := range errors {
			qe := QueryError{e["errors"]["message"]}
			g.Errors = append(g.Errors, qe)
		}
		return g
	}
	g.Data = responseData["data"].(map[string]interface{})
	return g
}
