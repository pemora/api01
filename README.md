# api01
go wrapper to run requests on 01 edu platform.

First, setup your environment:
```shell
export API01_GITEA_TOKEN="PUT YOUR GITEA TOKEN HERE"
```

then simply create a new client and run a query!
```go
package main

import (
        "fmt"

        "github.com/pemora/api01"
)

func main() {
        client, err := api01.NewClient("dakar.01-edu.org")
        if err != nil {
                fmt.Println("Error:", err)
                return
        }

        fmt.Printf("%#v\n\n", client.Token)
        piscines, err := client.GraphqlQuery(`
                query piscineList {
                        piscines: event (where: {path: {_eq: "/atos/piscine"}}) {
                       id
                              createdAt
                                    }
                        }`)
        if err != nil {
                fmt.Println(err)
                return
        }
        fmt.Printf("%+v\n", piscines)
}
```
