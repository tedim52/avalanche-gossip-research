package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ava-labs/coreth/ethclient"
)

func post(url string, payload string) string {
	request, error := http.NewRequest("POST", url, bytes.NewBuffer([]byte(payload)))
	if error != nil {
		panic(error)
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")

	client := &http.Client{}
	response, error := client.Do(request)
	if error != nil {
		panic(error)
	}
	defer response.Body.Close()

	//fmt.Println("response Status:", response.Status)
	//fmt.Println("response Headers:", response.Header)
	body, _ := ioutil.ReadAll(response.Body)
	//fmt.Println("response Body:", string(body))
	return string(body)
}

// TODO add nodes method
func start_nodes(n int, path string) {
	httpposturl := "http://localhost:8081/v1/control/start"

	var jsonData = fmt.Sprintf(`{
		"execPath": "%s",
		"numNodes": "%d",
		"logLevel":"INFO"
	}`, path, n)
	post(httpposturl, jsonData)
}

func add_node(x int, path string) {
	post("http://localhost:8081/v1/control/addnode", fmt.Sprintf(`{"name":"node%d","execPath":"%s","logLevel":"INFO"}`, x, path))
}

func pollNode(addr string) {
	client, err := ethclient.Dial(fmt.Sprintf("ws://%s/ext/bc/C/ws", addr))
	for err != nil {
		time.Sleep(1 * time.Second)
		fmt.Printf("Retrying %s\n", addr)
		client, err = ethclient.Dial(fmt.Sprintf("ws://%s/ext/bc/C/ws", addr))
	}

	hash_channel := make(chan *common.Hash)

	sub, err := client.SubscribeNewAcceptedTransactions(context.Background(), hash_channel)
	if err != nil {
		fmt.Println("sub failed", err)
	}

    fmt.Printf("Listening on %s\n", addr) 
	for {
	  select {
  	    case err := <-sub.Err():
    	      fmt.Println(err)
  	    case vLog := <-hash_channel:
    	      //fmt.Println(vLog) // pointer to event log
	      tx, _, err := client.TransactionByHash(context.Background(), *vLog) 
	      if err != nil {
		fmt.Println("tx lookup failed", err)
	      } else {
		fmt.Println(vLog, " => ", tx.FirstSeen())
	      }
      	}
	}
}

func checkHealth() bool {
	res := post("http://localhost:8081/v1/control/status", "")
	var result map[string]any
	json.Unmarshal([]byte(res), &result)
	clusterInfo := result["clusterInfo"].(map[string]any)
	health := clusterInfo["healthy"].(bool)
	return health
}

func getAddrs(n int) []string {
	res := post("http://localhost:8081/v1/control/status", "")
	var result map[string]any
	json.Unmarshal([]byte(res), &result)
	nodeInfo, ok := result["clusterInfo"].(map[string]any)["nodeInfos"].(map[string]any)
	if !ok {
		fmt.Println(result)
		panic("No nodeInfos")
	}
	addrs := make([]string, n)
	i := 0
	prefix := len("http://")
	for _, value := range nodeInfo {
		uri := value.(map[string]any)["uri"].(string)
		addrs[i] = uri[prefix:]
		i++
	}
	return addrs
}

func fundNetwork() {
	pword := "YY0*id@K#A29"
	// Create User
	url := "http://127.0.0.1:9650/ext/keystore"
	payload := fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id"     :1,
		"method" :"keystore.createUser",
		"params" :{
			"username": "username",
			"password": "%s"
		}
		`, pword)
	post(url, payload)
	
	// Import Key
	url = "http://127.0.0.1:9650/ext/bc/C/avax"
	payload = fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id"     : 1,
		"method" :"avm.importKey",
		"params" :{
			"username": "username",
			"password": "%s",
			"privateKey":"PrivateKey-ewoqjP7PxY4yr3iLTpLisriqt94hdyDFNgchSxGGztUrTXtNN"
		}
		`, pword)
	post(url, payload)
	
	// Check Balance
	url = "http://127.0.0.1:9650/ext/bc/C/rpc"
	payload = `{
		"jsonrpc":"2.0",
		"id"     : 1,
		"method" :"eth_getBalance",
		"params" :{
			"0x8db97C7cEcE249c2b98bDC0226Cc4C2A57BF52FC",
			"latest"
		}
	}`
	post(url, payload)
}

func main() {
	filename := os.Args[1]
	file, err := os.Open(filename)
	if err != nil {
        log.Fatalf("failed to open")
    }

	scanner := bufio.NewScanner(file)

	scanner.Split(bufio.ScanLines)
    
	total_nodes := 0

    for scanner.Scan() {
		line := strings.Split(scanner.Text(), " ")
        nodes, _ := strconv.Atoi(line[0])
		if total_nodes == 0 { // First time do batch launch
			total_nodes += nodes
			start_nodes(nodes, line[1])
		} else { // Add nodes one by one after (no batch available)
			for i := 0; i < nodes; i++ {
				total_nodes += 1
				add_node(total_nodes, line[1])
			}
		}
    }

	file.Close()

	fmt.Print("Waiting for cluster health...")
	for !checkHealth() {
		time.Sleep(5 * time.Second)
		fmt.Print(".")
	}
	fmt.Println("")

	// Get Addrs
	addrs := getAddrs(total_nodes)
	
	fmt.Println("Funding Network")
	fundNetwork()

	// double check port info
	for _, addr := range addrs {
        go pollNode(addr)
    }

	for {}
}