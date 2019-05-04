package main

import (
	"../cola/filestore"
	"../cola/filevault"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

var config *filestore.FileStore
var server_url string
var db *sql.DB
var fv *filevault.FileVault

func main() {
	var err error
	if err = LoadConfig(); err == nil {
		err = errors.New("Invalid or missing command.")
		args := os.Args
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
		fmt.Printf("\n ** ERROR: %s\n\n", err.Error())
		if err.Error() == "Invalid or missing command." {
			Usage()
		}
	}
}

func Check() (err error) {
	if db == nil {
		var resp *http.Response
		resp, err = http.Get(server_url + "/check")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
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
	args := os.Args
	if len(args) < 3 {
		err = errors.New("exist: No filename specified.")
		return
	}
	if db == nil {
		var resp *http.Response
		resp, err = http.Get(server_url + "/exist?fn=" + url.QueryEscape(args[2]))
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
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
	args := os.Args
	if len(args) < 3 {
		err = errors.New("export: No file_id specified.")
		return
	}
	file_id, _ := strconv.Atoi(args[2])
	if file_id == 0 {
		err = errors.New("extract: Invalid file_id.")
		return
	}
	var filename string
	if len(args) > 3 {
		filename = args[3]
	}
	if db == nil {
		if len(args) < 4 {
			err = errors.New("extract: No filename specified.")
			return
		}
		var resp *http.Response
		resp, err = http.Get(server_url + "/extract?f=" + args[2])
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
				err = errors.New("extract: " + resp.Status)
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
	args := os.Args
	if len(args) < 3 {
		err = errors.New("hash: No hash specified.")
		return
	}
	if db == nil {
		var resp *http.Response
		resp, err = http.Get(server_url + "/hash?h=" + url.QueryEscape(args[2]))
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
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
	args := os.Args
	if len(args) < 3 {
		err = errors.New("import: No file specified.")
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
		request, err = fileUploadRequest(server_url+"/import", params, "file", args[2])
		if err != nil {
			return
		}
		client := &http.Client{}
		var resp *http.Response
		resp, err = client.Do(request)
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
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
	args := os.Args
	if len(args) < 3 {
		err = errors.New("info: No file_id specified.")
		return
	}
	file_id, _ := strconv.Atoi(args[2])
	if file_id == 0 {
		err = errors.New("info: Invalid file_id.")
		return
	}
	if db == nil {
		var resp *http.Response
		resp, err = http.Get(server_url + "/info?f=" + args[2])
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
		}
	} else {
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
	}
	return
}

func List() (err error) {
	args := os.Args
	if len(args) < 3 {
		err = errors.New("list: No path specified.")
		return
	}
	if args[2][len(args[2])-1:] != "/" {
		err = errors.New("list: Path must end with '/'.")
		return
	}
	if db == nil {
		var resp *http.Response
		resp, err = http.Get(server_url + "/list?p=" + url.QueryEscape(args[2]))
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
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

func LoadConfig() (err error) {
	args := os.Args
	exe_path, exe_filename := filepath.Split(args[0])
	exe_ext := filepath.Ext(exe_filename)
	var config_filename string
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
	}
	return
}

func Query() (err error) {
	args := os.Args
	if len(args) < 2 {
		err = errors.New("query: No query terms specified.")
		return
	}
	var terms string
	for i := 2; i < len(args); i++ {
		terms += args[i] + " "
	}
	if db == nil {
		var resp *http.Response
		resp, err = http.Get(server_url + "/query?t=" + url.QueryEscape(terms))
		if err != nil {
			return
		}
		defer resp.Body.Close()
		var body []byte
		body, err = ioutil.ReadAll(resp.Body)
		if err == nil {
			fmt.Printf("%s", string(body))
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

func Usage() {
	args := os.Args
	fmt.Printf("fvclient v1.2\n")
	fmt.Printf("usage: %s <command> [arguments]\n", args[0])
	fmt.Printf("\n  commands:\n    check\n    exist <filename>\n    extract <file_id> <filename>\n    hash <hash>\n    import <file> [filename]\n    info <file_id>\n    list <path>\n    query <terms>\n\n")
}
