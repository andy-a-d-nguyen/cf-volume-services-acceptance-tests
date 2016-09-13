package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func main() {
	http.HandleFunc("/", hello)
	http.HandleFunc("/env", env)
	http.HandleFunc("/write", write)
	fmt.Println("listening...")

	ports := os.Getenv("PORT")
	portArray := strings.Split(ports, " ")

	errCh := make(chan error)

	for _, port := range portArray {
		println(port)
		go func(port string) {
			errCh <- http.ListenAndServe(":"+port, nil)
		}(port)
	}

	err := <-errCh
	if err != nil {
		panic(err)
	}
}

type VCAPApplication struct {
	InstanceIndex int `json:"instance_index"`
}

func hello(res http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(res, "instance index: %s", os.Getenv("INSTANCE_INDEX"))
}

func write(res http.ResponseWriter, req *http.Request) {
	vcapEnv := os.Getenv("VCAP_SERVICES")

	r, err := regexp.Compile("\"container_dir\": \"([^\"]+)\"")
	if err != nil {
		panic(err)
	}

	match := r.FindStringSubmatch(vcapEnv)
	if len(match) < 2 {
		fmt.Fprintf(os.Stderr, "VCAP_SERVICES is %s", vcapEnv)
		panic("failed to find container_dir in environment json")
	}

	mountPointPath := match[1] + "/test.txt"

	d1 := []byte("Hello Persistent World!\n")
	err = ioutil.WriteFile(mountPointPath, d1, 0644)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte("Writing \n"))
		res.Write([]byte(err.Error()))
		return
	}

	res.WriteHeader(http.StatusOK)
	body, err := ioutil.ReadFile(mountPointPath)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		res.Write([]byte("Reading \n"))
		res.Write([]byte(err.Error()))
		return
	}
	res.Write(body)
	return
}

func env(res http.ResponseWriter, req *http.Request) {
	for _, e := range os.Environ() {
		fmt.Fprintf(res, "%s\n", e)
	}
}
