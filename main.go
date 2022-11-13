package main

import (
	"context"
	"encoding/json"
	"fmt"
	"google.golang.org/api/option"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"google.golang.org/api/run/v1"
)

var (
	serviceName string
	cloudRun    *run.APIService
)

func main() {
	region := os.Getenv("REGION")
	if region == "" {
		log.Fatal("must specify REGION")
	}

	project := os.Getenv("PROJECT")
	if project == "" {
		log.Fatal("must specify PROJECT name/id")
	}

	serviceName = os.Getenv("SERVICE")
	if serviceName == "" {
		log.Fatal("must specify SERVICE name/id")
	}

	serviceName = fmt.Sprintf("namespaces/%s/services/%s", project, serviceName)

	var err error
	cloudRun, err = run.NewService(context.Background(), option.WithEndpoint("https://"+region+"-run.googleapis.com"))
	if err != nil {
		log.Fatal(fmt.Errorf("failed to initialize client: %w", err))
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Fatal(http.ListenAndServe(":"+port, http.HandlerFunc(handle)))
}

func handle(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		log.Print(fmt.Errorf("cannot read incoming request body: %w", err))
		return
	}

	var msg struct {
		Message struct {
			Data []byte
		}
	}
	if err = json.Unmarshal(b, &msg); err != nil {
		log.Print(fmt.Errorf("cannot parse outer JSON from incoming request body: %w", err))
		return
	}

	var evt struct {
		Action string
		Digest string
		Tag    string
	}
	if err = json.Unmarshal(msg.Message.Data, &evt); err != nil {
		log.Print(fmt.Errorf("cannot parse inner JSON from the message data: %w", err))
		return
	}
	if evt.Action != "INSERT" {
		log.Print(fmt.Errorf("action is not insert but %q instead", evt.Action))
		return
	}

	service, err := cloudRun.Namespaces.Services.Get(serviceName).Do()
	if err != nil {
		log.Print(fmt.Errorf("cannot fetch the service config: %w", err))
		return
	}

	specTag := strings.Split(service.Spec.Template.Spec.Containers[0].Image, "@")[0]
	if specTag != evt.Tag {
		log.Print(fmt.Errorf("push to the wrong tag (%q /= %q)", evt.Tag, specTag))
		return
	}

	service.Spec.Template.Spec.Containers[0].Image = evt.Tag + "@" + strings.Split(evt.Digest, "@")[1]
	service.Spec.Template.Metadata.Name = ""
	_, err = cloudRun.Namespaces.Services.ReplaceService(serviceName, service).Do()
	if err != nil {
		log.Print(fmt.Errorf("cannot update the service: %w", err))
		return
	}

	log.Printf("deployed an update with new image: " + service.Spec.Template.Spec.Containers[0].Image)
	_, _ = fmt.Fprintf(w, "Ok\n")
}
