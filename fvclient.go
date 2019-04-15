package main

import (
	"../cola/filestore"
	"bytes"
	"errors"
	"fmt"
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
			} else if command == "import" {
				err = Import()
			} else if command == "info" {
				err = Info()
			} else if command == "query" {
				err = Query()
			}
		}
	}
	if err != nil {
		fmt.Printf(" ** ERROR: %s\n", err.Error())
		if err.Error() == "Invalid or missing command." {
			Usage()
		}
	}
}

func Check() (err error) {
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
	return
}

func Exist() (err error) {
	args := os.Args
	if len(args) < 3 {
		err = errors.New("exist: No filename specified.")
		return
	}
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
	return
}

func Extract() (err error) {
	args := os.Args
	if len(args) < 3 {
		err = errors.New("extract: No file_id specified.")
		return
	}
	file_id, _ := strconv.Atoi(args[2])
	if file_id == 0 {
		err = errors.New("extract: Invalid file_id.")
		return
	}
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
		return
	}
	config := filestore.New(config_filename)
	server_url = config.Read("server_url")
	/*
	   root_path := config.Read("root_path")
	   if root_path == "" {
	       err = errors.New(config_filename + ": Invalid or missing root_path.")
	       return
	   }
	   db_type := config.Read("db_type")
	   if db_type != "mysql" {
	       err = errors.New(config_filename + ": Invalid or missing db_type.")
	       return
	   }
	   db_user := config.Read("db_user")
	   if db_user == "" {
	       err = errors.New(config_filename + ": Missing db_user.")
	       return
	   }
	   db_password := config.Read("db_password")
	   if db_password == "" {
	       err = errors.New(config_filename + ": Missing db_password.")
	       return
	   }
	   db_database := config.Read("db_database")
	   if db_database == "" {
	       err = errors.New(config_filename + ": Missing db_database.")
	       return
	   }
	   db_host := config.Read("db_host")
	   if db_host == "" {
	       err = errors.New(config_filename + ": Missing db_host.")
	       return
	   }
	   db_connect := db_user + ":" + db_password + "@tcp(" + db_host + ")/" + db_database
	   if db, err = sql.Open(db_type, db_connect); err == nil {
	       fv = filevault.New(db, root_path)
	   }
	*/
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
	return
}

func Usage() {
	args := os.Args
	fmt.Printf("usage: %s <command> [arguments]\n", args[0])
	fmt.Printf("\n  commands:\n    import <file> [filename]\n    extract <file_id> [filename]\n")
}
