package api01

import (
	"fmt"
	"os"
	"testing"
)

func TestClient(t *testing.T) {
	client, err := NewClient(os.Getenv("CAMPUS01ENDPOINT"))
	if err != nil {
		t.Fatalf("Can't create client:%s", err)
	}
	t.Log(client)

}

func TestBasicRequest(t *testing.T) {
	client, err := NewClient(os.Getenv("CAMPUS01ENDPOINT"))
	if err != nil {
		t.Fatalf("Can't create client: %s\n", err)
	}

	resp := client.GraphqlQuery("query users { user {login }	}", Vars{})

	if resp.HasErrors() {
		if len(resp.Errors) > 1 {
			errs := ""
			for _, e := range resp.Errors {
				errs = fmt.Sprintf("%s    - %v\n", errs, e)
			}
			t.Fatalf("Got multiples errors:%s\n", errs)
		}
		t.Fatalf("Error in response: %s", resp.Errors[0])
	}
}

func TestRequestWithVariables(t *testing.T) {
	client, err := NewClient(os.Getenv("CAMPUS01ENDPOINT"))
	if err != nil {
		t.Fatalf("Can't create client: %s\n", err)
	}

	variables := Vars{"login": os.Getenv("USER01LOGIN")}
	resp := client.GraphqlQuery("query users ($login:String!) { user(where: {login: {_eq: $login}}) {login } }", variables)

	if resp.HasErrors() {
		if len(resp.Errors) > 1 {
			errs := ""
			for _, e := range resp.Errors {
				errs = fmt.Sprintf("%s    - %v\n", errs, e)
			}
			t.Fatalf("Got multiples errors:%s\n", errs)
		}
		t.Fatalf("Error in response: %s", resp.Errors[0])
	}
}
