package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"time"

	"./service"
	res "github.com/jirenius/go-res"
	"github.com/jirenius/resgate/logger"
)

var stopTimeout = 10 * time.Second

var usageStr = `
Usage: rest2res [options]

Service Options:
    -n, --nats <url>                 NATS Server URL (default: nats://127.0.0.1:4222)
    -c, --config <file>              Configuration file (required)

Common Options:
    -h, --help                       Show this message
`

// Config holds server configuration
type Config struct {
	NatsURL        string `json:"natsUrl"`
	ExternalAccess bool   `json:"externalAccess"`
	Debug          bool   `json:"debug,omitempty"`
	service.Config
}

// SetDefault sets the default values
func (c *Config) SetDefault() {
	if c.NatsURL == "" {
		c.NatsURL = "nats://127.0.0.1:4222"
	}
	c.Config.SetDefault()
}

// Init takes a path to a json encoded file and loads the config
// If no file exists, a new file with default settings is created
func (c *Config) Init(fs *flag.FlagSet, args []string) error {
	var (
		showHelp   bool
		configFile string
	)

	fs.BoolVar(&showHelp, "h", false, "Show this message.")
	fs.BoolVar(&showHelp, "help", false, "Show this message.")
	fs.StringVar(&configFile, "c", "", "Configuration file.")
	fs.StringVar(&configFile, "config", "", "Configuration file.")
	fs.StringVar(&c.NatsURL, "n", "", "NATS Server URL.")
	fs.StringVar(&c.NatsURL, "nats", "", "NATS Server URL.")

	if err := fs.Parse(args); err != nil {
		printAndDie(fmt.Sprintf("error parsing arguments: %s", err.Error()), true)
	}

	if showHelp {
		usage()
	}

	if configFile == "" {
		printAndDie(fmt.Sprintf("missing config file"), true)
	}

	fin, err := ioutil.ReadFile(configFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("error loading config file: %s", err)
		}

		c.SetDefault()

		fout, err := json.MarshalIndent(c, "", "\t")
		if err != nil {
			return fmt.Errorf("error encoding config: %s", err)
		}

		ioutil.WriteFile(configFile, fout, os.FileMode(0664))
	} else {
		err = json.Unmarshal(fin, c)
		if err != nil {
			return fmt.Errorf("error parsing config file: %s", err)
		}

		// Overwrite configFile options with command line options
		fs.Parse(args)
	}

	// Any value not set, set it now
	c.SetDefault()

	// Set access granted access handler if no external authorization is used
	if !c.ExternalAccess {
		for i := range c.Config.Endpoints {
			c.Config.Endpoints[i].Access = res.AccessGranted
		}
	}

	return nil
}

// usage will print out the flag options for the server.
func usage() {
	fmt.Printf("%s\n", usageStr)
	os.Exit(0)
}

func printAndDie(msg string, showUsage bool) {
	fmt.Fprintf(os.Stderr, "%s\n", msg)
	if showUsage {
		fmt.Fprintf(os.Stderr, "%s\n", usageStr)
	}
	os.Exit(1)
}

func main() {
	fs := flag.NewFlagSet("rest2res", flag.ExitOnError)
	fs.Usage = usage

	var cfg Config
	err := cfg.Init(fs, os.Args[1:])
	if err != nil {
		printAndDie(err.Error(), false)
	}

	s, err := service.NewService(cfg.Config)
	if err != nil {
		printAndDie(err.Error(), false)
	}

	s.SetLogger(logger.NewStdLogger(cfg.Debug, cfg.Debug))

	// Start service in separate goroutine
	stop := make(chan bool)
	go func() {
		defer close(stop)
		if err := s.ListenAndServe("nats://localhost:4222"); err != nil {
			fmt.Printf("%s\n", err.Error())
		}
	}()

	// Wait for interrupt signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	select {
	case <-c:
		// Graceful stop
		s.Shutdown()
	case <-stop:
	}
}
