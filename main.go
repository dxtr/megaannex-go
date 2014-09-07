package main

import (
	"bufio"
	"errors"
	"fmt"
	gomega "github.com/t3rm1n4l/go-mega"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	VERSION = "0.01"
	AUTHOR  = "Kim Lidström"
	URL     = ""

	MEGA_API_URL         = "https://eu.api.mega.co.nz"
	MEGA_RETRIES         = 3
	MEGA_DOWNLOADWORKERS = 1
	MEGA_UPLOADWORKERS   = 1
	MEGA_TIMEOUT         = 30
	MEGA_ROOT            = "mega"
	MEGA_TRASH           = "trash"

	ANNEX_VERSION = 1
)

type megaclient struct {
}

var (
	mega       *gomega.Mega = nil
	rootfs     *gomega.Node = nil
	folderfs   *gomega.Node = nil
	username   string       = ""
	password   string       = ""
	folder     string       = ""
	pathsplit  *[]string    = nil
	encryption string       = ""
	scanner                 = bufio.NewScanner(os.Stdin)

	EINVALID_PATH  = errors.New("Invalid mega path")
	ENOT_FILE      = errors.New("Requested object is not a file")
	ENOT_DIRECTORY = errors.New("A non-directory exists at this path")
	EINVALID_SRC   = errors.New("Invalid source path")
	EFILE_EXISTS   = errors.New("File with same name already exists")
	EINVALID_DEST  = errors.New("Invalid destination path")
	EDIR_EXISTS    = errors.New("A directory with same name already exists")
)

type callback func([]string) bool

func progressBar(ch chan int, wg *sync.WaitGroup, size int64) {
	defer func() {
		wg.Done()
	}()
	bytesread := 0

	showProgress := func() {
		fmt.Printf("PROGRESS %d\n", bytesread)
	}

	for {
		b := 0
		ok := false

		select {
		case b, ok = <-ch:
			if ok == false {
				return
			}

		case <-time.After(time.Second * 10):
			showProgress()
			continue
		}
		bytesread += b
		showProgress()
	}
}

func buildPath(path string) string {
	return fmt.Sprintf("%s:/%s", MEGA_ROOT, path)
}

func getline() []string {
	if scanner.Scan() {
		input := scanner.Text()
		return strings.SplitN(input, " ", 2)
	}
	return nil
}

func setCreds(username string, password string) {
	fmt.Printf("SETCREDS mycreds %s %s\n", username, password)
}

func getCreds() bool {
	fmt.Printf("GETCREDS mycreds\n")
	tmp := getline()

	if tmp != nil && len(tmp) > 1 && tmp[0] == "CREDS" {
		unamepswd := strings.SplitN(tmp[1], " ", 2)

		if len(unamepswd) < 2 || unamepswd[0] == "" || unamepswd[1] == "" {
			return false
		}

		username = unamepswd[0]
		password = unamepswd[1]
		return true
	} else {
		return false
	}
}

func getConfig(conf string) string {
	val := ""
	if conf == "" {
		return val
	}

	fmt.Printf("GETCONFIG %s\n", conf)
	tmp := getline()

	if tmp != nil && len(tmp) > 1 && tmp[0] == "VALUE" {
		val = tmp[1]
	} else {
		val = ""
	}

	return val
}

func getDirHash(key string) string {
	fmt.Printf("DIRHASH %s\n", key)
	tmp := getline()

	if tmp != nil && len(tmp) > 1 && tmp[0] == "VALUE" && len(tmp[1]) > 0 {
		hash := tmp[1]
		return hash
	} else {
		return ""
	}
}

func getFullPath(key string) string {
	return fmt.Sprintf("%s:/%s/%s%s", MEGA_ROOT, folder, getDirHash(key), key)
}

func getLookupParams(resource string, fs *gomega.MegaFS) (*gomega.Node, *[]string, error) {
	resource = strings.TrimSpace(resource)
	args := strings.SplitN(resource, ":", 2)
	if len(args) != 2 || !strings.HasPrefix(args[1], "/") {
		return nil, nil, EINVALID_PATH
	}

	var root *gomega.Node
	var err error

	switch {
	case args[0] == MEGA_ROOT:
		root = fs.GetRoot()
	case args[0] == MEGA_TRASH:
		root = fs.GetTrash()
	default:
		return nil, nil, EINVALID_PATH
	}

	pathsplit := strings.Split(args[1], "/")[1:]
	l := len(pathsplit)

	if l > 0 && pathsplit[l-1] == "" {
		pathsplit = pathsplit[:l-1]
		l -= 1
	}

	if l > 0 && pathsplit[l-1] == "" {
		switch {
		case l == 1:
			pathsplit = []string{}
		default:
			pathsplit = pathsplit[:l-2]
		}
	}

	return root, &pathsplit, err
}

func gotCreds(args []string) bool {
	if len(args) < 2 {
		return false
	}

	unamepswd := strings.SplitN(args[1], " ", 2)
	username = unamepswd[0]
	password = unamepswd[1]

	return true
}

func mkpath(dstres string) error {
	var nodes []*gomega.Node
	var node *gomega.Node

	root, pathsplit, err := getLookupParams(fmt.Sprintf("%s:/%s", MEGA_ROOT, dstres), mega.FS)
	if err != nil {
		return err
	}
	if len(*pathsplit) > 0 {
		nodes, err = mega.FS.PathLookup(root, *pathsplit)
	} else {
		return nil
	}

	lp := len(*pathsplit)
	ln := len(nodes)

	if len(nodes) > 0 {
		node = nodes[ln-1]
	} else {
		node = root
	}

	switch {
	case err == nil:
		if node.GetType() != gomega.FOLDER {
			return ENOT_DIRECTORY
		}
		return nil
	case err == gomega.ENOENT:
		remaining := lp - ln
		for i := 0; i < remaining; i++ {
			name := (*pathsplit)[ln]
			node, err = mega.CreateDir(name, node)
			if err != nil {
				return err
			}
			ln += 1
		}
		err = nil

	default:
		return err
	}

	return nil
}

func prepare(args []string) bool {
	var err error
	var fail bool = false
	var failmsg string = ""
	encryption = getConfig("encryption")
	folder = getConfig("folder")
	//var nodes []*gomega.Node
	//var node *gomega.Node

	if mega != nil {
		failmsg = "mega instance is not nil"
		fail = true
	} else if !getCreds() {
		failmsg = "Couldn't fetch credentials"
		fail = true
	} else if folder == "" {
		failmsg = "Folder isn't set"
		fail = true
	} else {
		//tmpfolder[0] = folder
		mega = gomega.New()
		mega.SetAPIUrl(MEGA_API_URL)
		mega.SetRetries(MEGA_RETRIES)
		mega.SetTimeOut(time.Second * MEGA_TIMEOUT)
		err = mega.SetDownloadWorkers(MEGA_DOWNLOADWORKERS)
		err = mega.SetUploadWorkers(MEGA_UPLOADWORKERS)
		if err == gomega.EWORKER_LIMIT_EXCEEDED {
			fail = true
			failmsg = fmt.Sprintf("%s : %d <= %d", err, MEGA_UPLOADWORKERS, gomega.MAX_UPLOAD_WORKERS)
			fmt.Printf("PREPARE-FAILURE %s\n", failmsg)
			return !fail
		}

		err = mega.Login(username, password)
		if err != nil {
			fail = true
			failmsg = fmt.Sprintf("Couldn't log in! (Reason: %s)", err.Error())
			fmt.Printf("PREPARE-FAILURE %s\n", failmsg)
			return !fail
		}

		rootfs, pathsplit, err = getLookupParams(fmt.Sprintf("%s:/%s", MEGA_ROOT, folder), mega.FS)
		if err != nil {
			fail = true
			failmsg = fmt.Sprintf("%s", err)
			fmt.Printf("PREPARE-FAILURE %s\n", failmsg)
			return !fail
		}

		err = mkpath(folder)

		// func (m *Mega) CreateDir(name string, parent *Node) (*Node, error)
		// func (m *Mega) Delete(node *Node, destroy bool) error {
		// func (m *Mega) Rename(src *Node, name string) error {
		// func (m *Mega) Move(src *Node, parent *Node) error {
		// func (m *Mega) UploadFile(srcpath string, parent *Node, name string, progress *chan int) (*Node, error) {
		// func (m Mega) DownloadFile(src *Node, dstpath string, progress *chan int) error {
		// func (m *Mega) getFileSystem() error {
		// func (m *Mega) addFSNode(itm FSNode) (*Node, error) {
		// func (m Mega) GetUser() (UserResp, error) {
	}

	if fail {
		fmt.Printf("PREPARE-FAILURE %s\n", failmsg)
	} else {
		fmt.Println("PREPARE-SUCCESS")
	}

	return !fail
}

func initRemote(args []string) bool {
	//var err error
	var fail bool = false
	var failmsg string = ""
	uname := os.Getenv("MEGA_USERNAME")
	pswd := os.Getenv("MEGA_PASSWORD")

	if uname == "" || pswd == "" {
		fail = true
		failmsg = "Username and/or password isn't set. Set them with MEGA_USERNAME=\"username\" MEGA_PASSWORD=\"password\""
	} else {
		setCreds(uname, pswd)
	}

	if fail {
		fmt.Printf("INITREMOTE-FAILURE %s\n", failmsg)
	} else {
		fmt.Println("INITREMOTE-SUCCESS")
	}

	return !fail
}

func getAvailability(args []string) bool {
	fmt.Println("AVAILABILITY GLOBAL")
	return true
}

func checkPresent(args []string) bool {
	hash := args[1]
	//dirhash := getDirHash(hash)
	//dir := fmt.Sprintf("%s/%s", folder, dirhash)
	fullpath := getFullPath(hash)
	root, pathsplit, err := getLookupParams(fullpath, mega.FS)
	var nodes []*gomega.Node
	//var node *gomega.Node
	//var name string

	if err != nil {
		fmt.Printf("CHECKPRESENT-UNKNOWN %s %s\n", hash, err)
		return false
	} else if len(*pathsplit) > 0 {
		nodes, err = mega.FS.PathLookup(root, *pathsplit)
	}

	if err != nil {
		fmt.Printf("CHECKPRESENT-FAILURE %s\n", hash)
		return false
	}

	lp := len(*pathsplit)
	ln := len(nodes)
	if lp == ln {
		// If these are the same the file most likely exists
		fmt.Printf("CHECKPRESENT-SUCCESS %s\n", hash)
		return true
	}

	fmt.Printf("CHECKPRESENT-FAILURE %s\n", hash)
	return false
}

func transfer_store(key string, file string) bool {
	var nodes []*gomega.Node
	var node *gomega.Node
	var name string
	dirhash := getDirHash(key)
	fullpath := getFullPath(key)
	info, err := os.Stat(file)

	if err != nil {
		fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, err)
		return false
	}

	if info.Mode()&os.ModeType != 0 {
		fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, err)
		return false
	}

	err = mkpath(fmt.Sprintf("%s/%s", folder, dirhash))

	if err != nil {
		fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, err)
		return false
	}

	root, pathsplit, err := getLookupParams(fullpath, mega.FS)

	if len(*pathsplit) > 0 {
		nodes, err = mega.FS.PathLookup(root, *pathsplit)
	}

	if err != nil && err != gomega.ENOENT {
		fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, err)
		return false
	}

	lp := len(*pathsplit)
	ln := len(nodes)

	switch {
	case lp == ln+1 && ln > 0:
		node = nodes[ln-1]
	case lp == ln: // The file already exists?
		fmt.Printf("TRANSFER-SUCCESS STORE %s\n", key)
		return true
	case ln == 0 && lp == 1:
		node = root
	}

	name = path.Base(file)

	children, err := mega.FS.GetChildren(node)
	if err != nil {
		fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, err)
		return false
	}

	for _, c := range children {
		if c.GetName() == name {
			if info.Size() == c.GetSize() {
				fmt.Printf("TRANSFER-SUCCESS STORE %s\n", key)
				return true
			} else {
				fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, EFILE_EXISTS)
				return false
			}
		}
	}

	var ch *chan int
	var wg sync.WaitGroup
	ch = new(chan int)
	*ch = make(chan int)
	wg.Add(1)
	go progressBar(*ch, &wg, info.Size())

	_, err = mega.UploadFile(file, node, name, ch)
	wg.Wait()
	if err != nil {
		fmt.Printf("TRANSFER-FAILURE STORE %s %s\n", key, err)
		return false
	}

	fmt.Printf("TRANSFER-SUCCESS STORE %s\n", key)
	return true
}

func transfer_retrieve(key string, file string) bool {
	var nodes []*gomega.Node
	var node *gomega.Node
	fullpath := getFullPath(key)

	root, pathsplit, err := getLookupParams(fullpath, mega.FS)
	if err != nil {
		fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, err)
		return false
	}

	if len(*pathsplit) > 0 {
		nodes, err = mega.FS.PathLookup(root, *pathsplit)
	} else {
		err = EINVALID_PATH
	}

	if err != nil {
		fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, err)
		return false
	} else {
		node = nodes[len(nodes)-1]
		if node.GetType() != gomega.FILE {
			fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, ENOT_FILE)
			return false
		}
	}

	info, err := os.Stat(file)
	if os.IsNotExist(err) {
		d := path.Dir(file)
		_, err := os.Stat(d)
		if os.IsNotExist(err) {
			fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, EINVALID_DEST)
			return false
		}
	} else {
		if info.Mode().IsDir() {
			if strings.HasSuffix(file, "/") {
				file = path.Join(file, (*pathsplit)[len(*pathsplit)-1])
			} else {
				fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, EDIR_EXISTS)
				return false
			}
		}

		info, err = os.Stat(file)
		if os.IsNotExist(err) == false {
			if info.Size() == node.GetSize() {
				fmt.Printf("TRANSFER-SUCCESS RETRIEVE %s\n", key)
				return true
			} else {
				fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, EFILE_EXISTS)
				return false
			}
		}
		err = nil
	}

	var ch *chan int
	var wg sync.WaitGroup
	ch = new(chan int)
	*ch = make(chan int)
	wg.Add(1)
	go progressBar(*ch, &wg, node.GetSize())

	err = mega.DownloadFile(node, file, ch)
	wg.Wait()
	if err != nil {
		fmt.Printf("TRANSFER-FAILURE RETRIEVE %s %s\n", key, err)
		return false
	}

	fmt.Printf("TRANSFER-SUCCESS RETRIEVE %s\n", key)
	return true

	return true
}

func transfer(args []string) bool {
	params := strings.SplitN(args[1], " ", 3)
	method := params[0]
	key := params[1]
	file := params[2]

	if method == "STORE" {
		return transfer_store(key, file)
	} else if method == "RETRIEVE" {
		return transfer_retrieve(key, file)
	} else {
		fmt.Printf("TRANSFER-FAILURE %s %s Unknown method", method, key)
		return false
	}
}

func remove(args []string) bool {
	return true
}

var callbacks = map[string]callback{
	"PREPARE":         prepare,
	"INITREMOTE":      initRemote,
	"TRANSFER":        transfer,
	"CHECKPRESENT":    checkPresent,
	"REMOVE":          remove,
	"GETCOST":         nil,
	"GETAVAILABILITY": getAvailability,
}

func main() {
	fmt.Printf("VERSION %d\n", ANNEX_VERSION)

	for scanner.Scan() {
		input := strings.SplitN(scanner.Text(), " ", 2)
		if len(input) == 0 || len(input[0]) == 0 {
			continue
		}

		cb, ok := callbacks[input[0]]
		if ok == false || cb == nil {
			fmt.Println("UNSUPPORTED-REQUEST")
			continue
		} else {
			cb(input)
		}
	}
}
