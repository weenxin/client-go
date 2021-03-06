package jsonpath

import (
	"bytes"
	"fmt"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/client-go/util/jsonpath"
	"testing"
)

func TestJsonpathUsage(t *testing.T){
	templates := []string{"{@}","{@[0]}","{@[-1]}","{@[0:2]}","{@[0:4:2]}"}
	p := jsonpath.New("test")
	for _,template := range templates {
		err := p.Parse(template)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		buf := new(bytes.Buffer)
		bools := []bool{true,false,true,false}
		err = p.Execute(buf,bools)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		t.Logf("%v with %q parse result : %s", bools, template , buf.String())
	}


}

type book struct {
	Category string
	Author   string
	Title    string
	Price    float32
}

func (b book) String() string {
	return fmt.Sprintf("{Category: %s, Author: %s, Title: %s, Price: %v}", b.Category, b.Author, b.Title, b.Price)
}

type bicycle struct {
	Color string
	Price float32
	IsNew bool
}

type empName string
type job string
type store struct {
	Book      []book
	Bicycle   []bicycle
	Name      string
	Labels    map[string]int
	Employees map[empName]job
}


func TestStructInput(t *testing.T) {

	storeData := store{
		Name: "jsonpath",
		Book: []book{
			{"reference", "Nigel Rees", "Sayings of the Centurey", 8.95},
			{"fiction", "Evelyn Waugh", "Sword of Honour", 12.99},
			{"fiction", "Herman Melville", "Moby Dick", 8.99},
		},
		Bicycle: []bicycle{
			{"red", 19.95, true},
			{"green", 20.01, false},
		},
		Labels: map[string]int{
			"engieer":  10,
			"web/html": 15,
			"k8s-app":  20,
		},
		Employees: map[empName]job{
			"jason": "manager",
			"dan":   "clerk",
		},
	}


	p := jsonpath.New("test Strut")
	templates := []string{
		"hello {.Name}",
		"{.Labels.web/html}",
		"{.Book[*].Author}",
		"{range .Bicycle[*]}{ \"{\" }{ @.* }{ \"} \" }{end}"}
	for _,template := range templates {
		err := p.Parse(template)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		buf := new(bytes.Buffer)
		err = p.Execute(buf,storeData)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		t.Logf("%v with %q parse result : %s", storeData, template , buf.String())
	}

}

func TestJSONInput(t *testing.T) {
	var pointsJSON = []byte(`[
		{"id": "i1", "x":4, "y":-5},
		{"id": "i2", "x":-2, "y":-5, "z":1},
		{"id": "i3", "x":  8, "y":  3 },
		{"id": "i4", "x": -6, "y": -1 },
		{"id": "i5", "x":  0, "y":  2, "z": 1 },
		{"id": "i6", "x":  1, "y":  4 }
	]`)

	var pointsData interface{}
	err := json.Unmarshal(pointsJSON, &pointsData)
	if err != nil {
		t.Error(err)
	}
	templates := []string{
		"{[?(@.z)].id}", // ????????????z?????????
		"{[0]['id']}", //???0????????????id??????
	}
	p := jsonpath.New("test json")

	for _,template := range templates {
		err := p.Parse(template)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		buf := new(bytes.Buffer)
		err = p.Execute(buf,pointsData)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		t.Logf("%v with %q parse result : %s", pointsData, template , buf.String())
	}
}

// TestKubernetes tests some use cases from kubernetes
func TestKubernetes(t *testing.T) {
	var input = []byte(`{
	  "kind": "List",
	  "items":[
		{
		  "kind":"None",
		  "metadata":{
		    "name":"127.0.0.1",
			"labels":{
			  "kubernetes.io/hostname":"127.0.0.1"
			}
		  },
		  "status":{
			"capacity":{"cpu":"4"},
			"ready": true,
			"addresses":[{"type": "LegacyHostIP", "address":"127.0.0.1"}]
		  }
		},
		{
		  "kind":"None",
		  "metadata":{
			"name":"127.0.0.2",
			"labels":{
			  "kubernetes.io/hostname":"127.0.0.2"
			}
		  },
		  "status":{
			"capacity":{"cpu":"8"},
			"ready": true,
			"addresses":[
			  {"type": "LegacyHostIP", "address":"127.0.0.2"},
			  {"type": "another", "address":"127.0.0.3"}
			]
		  }
		}
	  ],
	  "users":[
	    {
	      "name": "myself",
	      "user": {}
	    },
	    {
	      "name": "e2e",
	      "user": {"username": "admin", "password": "secret"}
	  	}
	  ]
	}`)
	var nodesData interface{}
	err := json.Unmarshal(input, &nodesData)
	if err != nil {
		t.Error(err)
	}

	p := jsonpath.New("test json")
	templates := []string{
		"{range .items[*]}{.metadata.name}, {end}{.kind}", // ???????????????????????????metadata????????????????????????kind
		"{range .items[*]}{.metadata.name}{\"\\t\"}{end}", // ??????tab???
		"{.items[*].status.addresses[*].address}", //??????item???status?????????address???address
		"{range .items[*]}{range .status.addresses[*]}{.address}, {end}{end}",//??????????????????address
		"{.items[*].metadata.name}",//??????item?????????
		`{.items[*]['metadata.name', 'status.capacity']}`, //??????item????????????????????????????????????????????????????????????capacity
		`{range .items[*]}[{.metadata.name}, {.status.capacity}] {end}`,//???????????????????????????????????????????????????
		`{.users[?(@.name=="e2e")].user.password}`, //???????????????username???e2e????????????????????????
		`{.items[0].metadata.labels.kubernetes\.io/hostname}`, //???????????????item???kubernetes.io/hostname???LabelValue???
		`{.items[?(@.metadata.labels.kubernetes\.io/hostname=="127.0.0.1")].kind}`,//??????item???kubernetes.io/hostname???LabelValue????????????????????????kind
		`{.items[?(@..ready==true)].metadata.name}`,//????????????ready???true???item???????????????metadata???name
	}
	for _,template := range templates {
		err := p.Parse(template)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		buf := new(bytes.Buffer)
		err = p.Execute(buf,nodesData)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}

		t.Logf(" %q parse result : %s", template , buf.String())
	}


}

