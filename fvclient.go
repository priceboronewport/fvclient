/*
 *  fvclient - Filevault command line client.
 *
 *  Copyright (c) 2019  Priceboro Newport, Inc.  All Rights Reserved.
 *
 *  4/14/2019 - Version 1.0 - Initial Version
 *  6/13/2019 - Version 2.0 - Added Authentication & https
 *  6/14/2019 - Version 2.1 - Added --config= command line flag
 *  6/17/2019 - Version 2.2 - Fixed error display from fvserver
 *
 */

package main

import (
	"../cola/filestore"
	"../cola/filevault"
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io"
	"io/ioutil"
	"math/rand"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const version = "2.2"

var config *filestore.FileStore
var server_url string
var server_user string
var server_password string
var db *sql.DB
var fv *filevault.FileVault
var http_client *http.Client

func main() {
	var err error
	if err = LoadFlags(); err == nil {
		err = errors.New("Invalid or missing command.")
		args := Args()
		if len(args) > 1 {
			command := args[1]
			if command == "check" {
				err = Check()
			} else if command == "exist" {
				err = Exist()
			} else if command == "extract" {
				err = Extract()
			} else if command == "hash" {
				err = Hash()
			} else if command == "import" {
				err = Import()
			} else if command == "info" {
				err = Info()
			} else if command == "list" {
				err = List()
			} else if command == "query" {
				err = Query()
			}
		}
	}
	if err != nil {
		error := err.Error()
		fmt.Printf("\n ** ERROR: %s\n\n", error)
		if error == "Invalid or missing command." || (len(error) > 13 && error[0:13] == "Invalid flag ") {
			Usage()
		}
	}
}

func Args() []string {
	var result []string
	args := os.Args
	for _, v := range args {
		if v[0:1] != "-" {
			result = append(result, v)
		}
	}
	return result
}

func Auth(id string) string {
	salt := fmt.Sprintf("%d", rand.Uint32())
	return server_user + "/" + salt + "/" + SHA256(id+salt+server_password)
}

func Check() (err error) {
	if db == nil {
		var resp *http.Response
		server_url += "/check?auth=" + Auth("check")
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		var results []string
		results, err = fv.Check()
		if err == nil {
			if len(results) > 0 {
				for _, v := range results {
					fmt.Printf("%s\n", v)
				}
			} else {
				fmt.Printf("No errors.\n")
			}
		}
	}
	return
}

func Exist() (err error) {
	args := Args()
	if len(args) < 3 {
		err = errors.New("No filename specified.")
		return
	}
	if db == nil {
		var resp *http.Response
		server_url += "/exist?auth=" + Auth("exist "+args[2]) + "&fn=" + url.QueryEscape(args[2])
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		var file_ids []int
		file_ids, err = fv.QueryFilename(args[2])
		if err == nil {
			for _, file_id := range file_ids {
				fmt.Printf("%10d\n", file_id)
			}
		}
	}
	return
}

func Extract() (err error) {
	args := Args()
	if len(args) < 3 {
		err = errors.New("No file_id specified.")
		return
	}
	file_id, _ := strconv.Atoi(args[2])
	if file_id == 0 {
		err = errors.New("Invalid file_id.")
		return
	}
	var filename string
	if len(args) > 3 {
		filename = args[3]
	}
	if db == nil {
		if len(args) < 4 {
			err = errors.New("No filename specified.")
			return
		}
		var resp *http.Response
		server_url += "/extract?auth=" + Auth("extract "+args[2]) + "&f=" + args[2]
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			var f *os.File
			f, err = os.OpenFile(args[3], os.O_WRONLY|os.O_CREATE, 0664)
			if err != nil {
				return
			}
			defer f.Close()
			_, err = io.Copy(f, resp.Body)
		} else {
			var body []byte
			body, err = ioutil.ReadAll(resp.Body)
			if string(body) != "" {
				err = errors.New(string(body))
			} else {
				err = errors.New(resp.Status)
			}
		}
	} else {
		filename, err = fv.Extract(file_id, filename)
		if err == nil {
			fmt.Printf("%10d: %s\n", file_id, filename)
		}
	}
	return
}

func fileUploadRequest(uri string, params map[string]string, param_name string, path string) (*http.Request, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(param_name, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(part, file)

	for key, val := range params {
		_ = writer.WriteField(key, val)
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", uri, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req, err
}

func Hash() (err error) {
	args := Args()
	if len(args) < 3 {
		err = errors.New("No hash specified.")
		return
	}
	if db == nil {
		var resp *http.Response
		server_url += "/hash?auth=" + Auth("hash "+args[2]) + "&h=" + url.QueryEscape(args[2])
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		var file_ids []int
		var filenames []string
		file_ids, filenames, err = fv.ListHash(args[2])
		if err == nil {
			for i := 0; i < len(file_ids); i++ {
				fmt.Printf("%10d: %s\n", file_ids[i], filenames[i])
			}
		}
	}
	return
}

func Import() (err error) {
	args := Args()
	if len(args) < 3 {
		err = errors.New("No file specified.")
		return
	}
	filename := args[2]
	if len(args) >= 4 {
		filename = args[3]
	}
	if db == nil {
		var fi os.FileInfo
		fi, err = os.Stat(args[2])
		if err != nil {
			return
		}
		params := map[string]string{
			"fn": filename,
			"ts": fi.ModTime().Format("2006-01-02 15:04:05")}
		var request *http.Request
		request, err = fileUploadRequest(server_url+"/import?auth="+Auth("import "+filename), params, "file", args[2])
		if err != nil {
			return
		}
		var resp *http.Response
		if http_client == nil {
			client := &http.Client{}
			resp, err = client.Do(request)
		} else {
			resp, err = http_client.Do(request)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		var fi os.FileInfo
		fi, err = os.Stat(args[2])
		if err != nil {
			return
		}
		file_id := 0
		file_id, err = fv.Import(args[2], filename, fi.ModTime())
		if err == nil {
			fmt.Printf("%10d: %s\n", file_id, filename)
		} else if err.Error() == "Exists" {
			fmt.Printf("%10d+ %s\n", file_id, filename)
			err = nil
		}
	}
	return
}

func Info() (err error) {
	args := Args()
	file_id := 0
	if len(args) > 2 {
		file_id, _ = strconv.Atoi(args[2])
		if file_id == 0 {
			err = errors.New("Invalid file_id.")
			return
		}
	}
	if db == nil {
		var resp *http.Response
		var auth string
		if len(args) > 2 {
			auth = Auth("info " + args[2])
		} else {
			auth = Auth("info")
		}
		server_url += "/info?auth=" + auth
		if len(args) > 2 {
			server_url += "&f=" + args[2]
		}
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		if file_id != 0 {
			var fi filevault.FileInfo
			fi, err = fv.Info(file_id)
			if err == nil {
				fmt.Printf("File ID: %d\n", fi.FileID)
				fmt.Printf("Path: %s\n", fi.Path)
				fmt.Printf("Name: %s\n", fi.Name)
				fmt.Printf("Date: %s\n", fi.Timestamp.Format("2006-01-02 15:04:05"))
				fmt.Printf("Size: %d\n", fi.Size)
				fmt.Printf("Hash: %s\n", fi.Hash)
			}
		} else {
			fmt.Printf("Filevault Client %s\n", version)
		}
	}
	return
}

func List() (err error) {
	args := Args()
	if len(args) < 3 {
		err = errors.New("No path specified.")
		return
	}
	if args[2][len(args[2])-1:] != "/" {
		err = errors.New("Path must end with '/'.")
		return
	}
	if db == nil {
		var resp *http.Response
		server_url += "/list?auth=" + Auth("list "+args[2]) + "&p=" + url.QueryEscape(args[2])
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		var file_ids []int
		var names []string
		file_ids, names, err = fv.ListPath(args[2])
		if err == nil {
			for i := 0; i < len(file_ids); i++ {
				fmt.Printf("%10d: %s\n", file_ids[i], names[i])
			}
		}
	}
	return
}

func LoadConfig(config_filename string) (err error) {
	config := filestore.New(config_filename)
	server_url = config.Read("server_url")
	if server_url == "" {
		db_type := config.Read("db_type")
		if db_type == "" {
			err = errors.New(config_filename + ": Missing server_url or db_type.")
			return
		}
		db_connect := config.Read("db_connect")
		if db_connect == "" {
			err = errors.New(config_filename + ": Missing db_connect.")
			return
		}
		root_path := config.Read("root_path")
		if root_path == "" {
			err = errors.New(config_filename + ": Missing root_path.")
			return
		}
		if db, err = sql.Open(db_type, db_connect); err == nil {
			fv = filevault.New(db, root_path)
		}
	} else {
		auth := strings.Split(config.Read("server_auth"), "/")
		if len(auth) < 2 {
			err = errors.New(config_filename + ": Missing or Invalid server_auth.")
			return
		}
		server_user = auth[0]
		server_password = auth[1]
		if config.Read("server_verify_certificate") == "" {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
			http_client = &http.Client{Transport: tr}
		}
	}
	return
}

func LoadFlags() (err error) {
	var flags []string
	args := os.Args
	for _, v := range args {
		if v[0:1] == "-" {
			flags = append(flags, v)
		}
	}
	var config_filename string
	for _, v := range flags {
		if (len(v) >= 10) && (v[0:9] == "--config=") {
			config_filename = v[9:]
		} else {
			err = errors.New("Invalid flag " + v)
			return
		}
	}
	if config_filename == "" {
		args := Args()
		exe_path, exe_filename := filepath.Split(args[0])
		exe_ext := filepath.Ext(exe_filename)
		if exe_ext != "" {
			config_filename = exe_path + exe_filename[:len(exe_filename)-len(exe_ext)] + ".conf"
		} else {
			config_filename = exe_path + exe_filename + ".conf"
		}
		if _, err = os.Stat(config_filename); err != nil {
			config_filename = "/etc/fvclient.conf"
			if _, err = os.Stat(config_filename); err != nil {
				return
			}
		}
	}
	return LoadConfig(config_filename)
}

func Query() (err error) {
	args := Args()
	if len(args) < 2 {
		err = errors.New("No query terms specified.")
		return
	}
	var terms string
	for i := 2; i < len(args); i++ {
		terms += args[i] + " "
	}
	if db == nil {
		var resp *http.Response
		server_url += "/query?auth=" + Auth("query "+terms) + "&t=" + url.QueryEscape(terms)
		if http_client == nil {
			resp, err = http.Get(server_url)
		} else {
			resp, err = http_client.Get(server_url)
		}
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			if resp.StatusCode == 200 {
				fmt.Printf("%s", string(body))
			} else {
				err = errors.New(string(body))
			}
		}
	} else {
		var file_ids []int
		var filenames []string
		file_ids, filenames, err = fv.Query(terms)
		for i := 0; i < len(file_ids); i++ {
			fmt.Printf("%10d: %s\n", file_ids[i], filenames[i])
		}
	}
	return
}

func SHA256(str string) string {
	h := sha256.New()
	h.Write([]byte(str))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func Usage() {
	args := Args()
	fmt.Printf("Filevault Client %s\n\n", version)
	fmt.Printf("usage: %s [flags] <command> [arguments]\n", args[0])
	fmt.Printf("\n  flags:\n    --config=file - Override default config.\n")
	fmt.Printf("\n  commands:\n    check\n    exist <filename>\n    extract <file_id> <filename>\n    hash <hash>\n    import <file> [filename]\n    info [file_id]\n    list <path>\n    query <terms>\n\n")
}
