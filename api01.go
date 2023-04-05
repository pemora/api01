package api01

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	env "github.com/Netflix/go-env"
	"github.com/golang-jwt/jwt/v5"
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

// Parse & unmarshal raw bytes into a token
func parseToken(rawToken []byte) (jwt.Claims) {
	token, _ := jwt.Parse(string(rawToken), func(token *jwt.Token) (interface{}, error){ 
		return []byte(""), nil
	})
	return token.Claims
}

//The client object used to requests to graphql
type Client struct {
	sync.Mutex
	Endpoint   string
	GiteaToken string `env:"API01_GITEA_TOKEN"`
	RawToken   []byte
	Token     jwt.Claims 
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

	token := parseToken(body)
	c.Token = token
	return c, err
}

// Sleep until token is expired and fetches a new one, rince, repeat ;)
//TODO: test !
func (c *Client) RefreshLoop() error {
	for true {
		now := time.Now()
		sec := now.Unix()
		exp, err := c.Token.GetExpirationTime()
		if err != nil {
			return err
		}
		_, _ = sec, exp
		time.Sleep(time.Second * exp.Sub(now))
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
	c.Token = parseToken(body)
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
