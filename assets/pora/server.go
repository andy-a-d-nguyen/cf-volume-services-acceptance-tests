package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	http.HandleFunc("/", hello)
	http.HandleFunc("/env", env)
	http.HandleFunc("/write", write)
	http.HandleFunc("/create", createFile)
	http.HandleFunc("/loadtest", dataLoad)
	http.HandleFunc("/loadtestcleanup", dataLoadCleanup)
	http.HandleFunc("/read/", readFile)
	http.HandleFunc("/chmod/", chmodFile)
	http.HandleFunc("/delete/", deleteFile)
	fmt.Println("listening...")

	ports := os.Getenv("PORT")
	portArray := strings.Split(ports, " ")

	errCh := make(chan error)

	for _, port := range portArray {
		go func(port string) {
			server := &http.Server{
				Addr:              fmt.Sprintf(":%s", port),
				Handler:           nil,
				ReadHeaderTimeout: 5 * time.Second,
			}
			errCh <- server.ListenAndServe()
		}(port)
	}

	err := <-errCh
	if err != nil {
		panic(err)
	}
}

func hello(res http.ResponseWriter, req *http.Request) {
	fmt.Fprintf(res, "instance index: %s", os.Getenv("INSTANCE_INDEX"))
}

func getPath() string {
	r, err := regexp.Compile(`"container_dir":\s*"([^"]+)"`)
	if err != nil {
		panic(err)
	}

	vcapEnv := os.Getenv("VCAP_SERVICES")
	match := r.FindStringSubmatch(vcapEnv)
	if len(match) < 2 {
		fmt.Fprintf(os.Stderr, "VCAP_SERVICES is %s", vcapEnv)
		panic("failed to find container_dir in environment json")
	}

	return match[1]
}

func write(res http.ResponseWriter, req *http.Request) {
	mountPointPath := getPath() + "/poratest-" + randomString(10)

	d1 := []byte("Hello Persistent World!\n")
	err := os.WriteFile(mountPointPath, d1, 0644)
	if err != nil {
		writeError(res, "Writing \n", err)
		return
	}

	f, err := os.OpenFile(mountPointPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		writeError(res, "Opening file to append \n", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString("Append text!\n"); err != nil {
		writeError(res, "Appending \n", err)
		return
	}

	body, err := os.ReadFile(mountPointPath)
	if err != nil {
		writeError(res, "Reading \n", err)
		return
	}

	err = os.Remove(mountPointPath)
	if err != nil {
		writeError(res, "Deleting \n", err)
		return
	}

	res.WriteHeader(http.StatusOK)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write(body)
	return
}

func dataLoad(res http.ResponseWriter, req *http.Request) {
	// this method will read and write data to a single file for 4 seconds, then clean up.
	mountPointPath := getPath() + "/poraload-" + randomString(10)

	d1 := []byte("Hello Persistent World!\n")
	err := os.WriteFile(mountPointPath, d1, 0644)
	if err != nil {
		writeError(res, "Writing \n", err)
		return
	}

	var totalIO int
	for startTime := time.Now(); time.Since(startTime) < 4*time.Second; {
		d2 := []byte(randomString(1048576))
		err := os.WriteFile(mountPointPath, d2, 0644)
		if err != nil {
			writeError(res, "Writing Load\n", err)
			return
		}
		body, err := os.ReadFile(mountPointPath)
		if err != nil {
			writeError(res, "Reading Load\n", err)
			return
		}
		if string(body) != string(d2) {
			writeError(res, "Data Mismatch\n", err)
			return
		}
		totalIO = totalIO + 1
	}

	err = os.Remove(mountPointPath)
	if err != nil {
		writeError(res, "Deleting\n", err)
		return
	}

	res.WriteHeader(http.StatusOK)
	body := fmt.Sprintf("%d MiB written\n", totalIO)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte(body))
	return
}

func dataLoadCleanup(res http.ResponseWriter, req *http.Request) {
	// this method will clean up any files that couldn't be deleted during load testing due to interruptions.
	mountPointPath := getPath() + "/poraload-*"

	files, err := filepath.Glob(mountPointPath)
	if err != nil {
		writeError(res, "Unable to find files \n", err)
		return
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil {
			writeError(res, "Unable to remove "+f+" \n", err)
			return
		}
	}

	res.WriteHeader(http.StatusOK)
	body := fmt.Sprintf("%d Files Removed\n", len(files))
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte(body))
	return
}

func createFile(res http.ResponseWriter, _ *http.Request) {
	fileName := "pora" + randomString(10)
	mountPointPath := filepath.Join(getPath(), fileName)

	d1 := []byte("Hello Persistent World!\n")
	err := os.WriteFile(mountPointPath, d1, 0644)
	if err != nil {
		writeError(res, "Writing \n", err)
		return
	}

	res.WriteHeader(http.StatusOK)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte(fileName))
	return
}

func readFile(res http.ResponseWriter, req *http.Request) {
	parts := strings.Split(req.URL.Path, "/")
	fileName := parts[len(parts)-1]
	mountPointPath := filepath.Join(getPath(), fileName)

	body, err := os.ReadFile(mountPointPath)
	if err != nil {
		res.WriteHeader(http.StatusNotFound)
		// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
		res.Write([]byte(err.Error()))
		return
	}

	res.WriteHeader(http.StatusOK)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write(body)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte("instance index: " + os.Getenv("INSTANCE_INDEX")))
	return
}

func chmodFile(res http.ResponseWriter, req *http.Request) {
	parts := strings.Split(req.URL.Path, "/")
	fileName := parts[len(parts)-2]
	mountPointPath := filepath.Join(getPath(), fileName)
	mode := parts[len(parts)-1]
	parsedMode, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		res.WriteHeader(http.StatusBadRequest)
		// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
		res.Write([]byte(err.Error()))
	}
	err = os.Chmod(mountPointPath, os.FileMode(parsedMode))
	if err != nil {
		res.WriteHeader(http.StatusForbidden)
		// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
		res.Write([]byte(err.Error()))
		return
	}

	res.WriteHeader(http.StatusOK)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte(fileName + "->" + mode))
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte("instance index: " + os.Getenv("INSTANCE_INDEX")))
	return
}

func deleteFile(res http.ResponseWriter, req *http.Request) {
	parts := strings.Split(req.URL.Path, "/")
	fileName := parts[len(parts)-1]
	mountPointPath := filepath.Join(getPath(), fileName)

	err := os.Remove(mountPointPath)
	if err != nil {
		res.WriteHeader(http.StatusForbidden)
		// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
		res.Write([]byte(err.Error()))
		return
	}

	res.WriteHeader(http.StatusOK)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte("deleted " + fileName))
	return
}

func env(res http.ResponseWriter, req *http.Request) {
	for _, e := range os.Environ() {
		fmt.Fprintf(res, "%s\n", e)
	}
}

func randomString(n int) string {
	runes := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	b := make([]rune, n)
	for i := range b {
		b[i] = runes[rand.Intn(len(runes))]
	}
	return string(b)
}

func writeError(res http.ResponseWriter, msg string, err error) {
	res.WriteHeader(http.StatusInternalServerError)
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte(msg))
	// #nosec - ignore errors writing http responses to avoid spamming logs in the event of a DoS
	res.Write([]byte(err.Error()))
}
