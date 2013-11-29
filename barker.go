package main

import (
  "bytes"
  "encoding/json"
  "encoding/base64"
  "flag"
  "fmt"
  "io/ioutil"
  "log"
  "net/http"
  "os/exec"
  "strings"
)

type MethodMap map[string]bool

type Task struct {
  Name, Cmd, Dir string
  Payload bool
  Methods []string
  HandledMethods MethodMap
}

func (task *Task) Run(payload []byte) bool {
  log.Printf("Running task '%s'", task.Name)
  cmd := exec.Command(task.Cmd)
  if len(task.Dir) != 0 {
    cmd.Dir = task.Dir
  }
  if task.Payload {
    cmd.Stdin = bytes.NewReader(payload)
  }
  var stderr bytes.Buffer
  cmd.Stderr = &stderr
  err := cmd.Run()
  if err != nil {
    log.Printf("Error running task '%s': %s", task.Name, stderr.String())
    return false
  }
  return true
}

type TaskList map[string]*Task

type Tasks struct {
  File string
  List TaskList
}

func NewTasksFromFile(file string) *Tasks {
  fileContents, err := ioutil.ReadFile(file)
  if err != nil {
    log.Fatal("Invalid task file.")
  }
  tasks := Tasks{file, make(TaskList)}
  json.Unmarshal(fileContents, &tasks.List)
  for name, task := range tasks.List {
    task.Name = name
    if len(task.Cmd) == 0 {
      log.Fatalf("No command specified for task '%s'", name)
    }
    if len(task.Methods) == 0 {
      task.Methods = append(task.Methods, "GET")
    }
    task.HandledMethods = make(MethodMap)
    for _, method := range task.Methods {
      task.HandledMethods[method] = true
    }
  }
  return &tasks
}

func route(tasks *Tasks, w http.ResponseWriter, r *http.Request) {
  path := strings.Trim(r.URL.Path, "/")
  if path == "__reload__" {
    log.Println("Reloading tasks")
    *tasks = *NewTasksFromFile(tasks.File)
    fmt.Fprint(w, "OK")
  } else if task, ok := tasks.List[path]; ok {
    if _, ok := task.HandledMethods[r.Method]; ok {
      var payload []byte
      if task.Payload {
        r.ParseForm()
        payload, _ = json.Marshal(r.Form)
      }
      if ok := task.Run(payload); ok {
        fmt.Fprint(w, "OK")
      } else {
        http.Error(w, "Cannot perform task.", 502)
      }
    } else {
      http.Error(w, "Method Not Allowed", 405)
    }
  } else {
    http.Error(w, "Not found.", 404)
  }
}

func main() {
  var auth, port, taskFile string
  flag.StringVar(&auth, "auth", "", "Basic auth credentials.")
  flag.StringVar(&port, "port", ":8080", "The port to run on.")
  flag.StringVar(&taskFile, "tasks", "./tasks.json", "The task file to use.")
  flag.Parse()
  var tasks Tasks
  tasks = *NewTasksFromFile(taskFile)
  if len(auth) > 0 {
    auth = string(base64.StdEncoding.EncodeToString([]byte(auth)))
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
      authHeader := r.Header["Authorization"]
      if len(authHeader) > 0 && strings.Fields(authHeader[0])[1] == auth {
        route(&tasks, w, r)
      }  else {
        w.Header().Set("WWW-Authenticate", "Basic realm=\"user\")")
        http.Error(w, "Unauthorized.", 401)
      }
    })
  } else {
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
      route(&tasks, w, r)
    })
  }
  log.Printf("Starting server at port %s", port)
  http.ListenAndServe(port, nil)
}
