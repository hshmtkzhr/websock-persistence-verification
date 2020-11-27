package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/gorilla/websocket"
)

var options struct {
	mode        string
	binding     string
	origin      string
	affinity_id string
	//printVersion bool
	//insecure bool //wss not supported so far
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "ws OPTIONS",
		Short: "websocket tool",
		Run: func(cmd *cobra.Command, args []string) {
			if options.mode == "server" && options.binding != "" {
				serverRoot(options.binding)
			} else if options.mode == "client" && options.origin != "" {
				clientRoot(options.origin, options.affinity_id)
			} else {
				cmd.Help()
				os.Exit(1)
			}
		},
	}
	rootCmd.Flags().StringVarP(&options.mode, "mode", "m", "", "server or client mode")
	rootCmd.Flags().StringVarP(&options.binding, "bind", "b", "", "address:port for binding")
	rootCmd.Flags().StringVarP(&options.origin, "origin", "o", "", "websocket origin")
	rootCmd.Flags().StringVarP(&options.affinity_id, "affinity-id", "i", "", "affinity id for server sticky")
	//rootCmd.Flags().BoolVarP(&options.printVersion, "version", "v", false, "print version")
	//rootCmd.Flags().BoolVarP(&options.insecure, "insecure", "k", false, "skip ssl certificate check")

	rootCmd.Execute()
}

func serverRoot(binding string) {
	http.HandleFunc("/echo", echo)
	log.Fatal(http.ListenAndServe(binding, nil))
}

var upgrader = websocket.Upgrader{} // use default options

func echo(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	defer c.Close()

	for {
		mt, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			break
		}
		log.Printf("recv: %s", message)
		err = c.WriteMessage(mt, message)
		if err != nil {
			log.Println("write:", err)
			break
		}
	}
}

func clientRoot(origin, affinity_id string) {
	log.SetFlags(0)

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: origin, Path: "/echo"}
	log.Printf("connecting to %s", u.String())

	c := &websocket.Conn{}
	err := errors.New("")
	if affinity_id == "" {
		c, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
	} else {
		header := http.Header{}
		header.Set("WS-Persistence", affinity_id)
		c, _, err = websocket.DefaultDialer.Dial(u.String(), header)
	}
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			log.Printf("recv: %s", message)
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
			if err != nil {
				log.Println("write:", err)
				return
			}
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
