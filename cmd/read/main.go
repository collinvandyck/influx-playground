package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
)

func main() {
	ctx := context.Background()
	err := run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

var (
	orgName    = "ngrok"
	bucketName = "data"
	token      = ""
	url        = "http://localhost:8086"
	debug      = false
	statName   = "stat"
)

type writeOpts struct {
	bucket      string
	measurement string
	numPoints   int
	tags        map[string]string
}

type writeStats struct {
	opts     writeOpts
	err      error
	duration time.Duration
}

func newClient() influxdb2.Client {
	influxClient := influxdb2.NewClientWithOptions(url, token, influxdb2.DefaultOptions().
		SetUseGZip(true))
	return influxClient
}

func run(ctx context.Context) error {
	if t := os.Getenv("TOKEN"); t != "" {
		token = t
	}

	flag.StringVar(&url, "url", url, "")
	flag.StringVar(&token, "token", token, "")
	flag.Parse()

	err := read(ctx)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	return nil
}

func read(ctx context.Context) error {
	influxClient := newClient()
	fmt.Println("Reading records")
	start := time.Now()
	count := 0
	res, err := influxClient.QueryAPI(orgName).QueryWithParams(context.Background(), `
		from(bucket: "`+bucketName+`")
			|> range(start: -1h) 
			|> filter(fn: (r) => r._measurement == "stat0")
			|> filter(fn: (r) => r._field == "log")
	`, map[string]string{})
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	for res.Next() {
		count++
		record := res.Record()
		if record.Field() == "log" {
			fmt.Printf("> %v\n", record.Value())
		}
	}
	if err = res.Err(); err != nil {
		return fmt.Errorf("res err: %w", err)
	}
	fmt.Printf("Read %d records in %v\n", count, time.Since(start).Truncate(time.Millisecond))
	return nil
}
