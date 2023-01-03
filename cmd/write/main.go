package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxWrite "github.com/influxdata/influxdb-client-go/v2/api/write"
	"github.com/influxdata/influxdb-client-go/v2/domain"
)

func main() {
	ctx := context.Background()
	err := run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

var (
	orgName         = "ngrok"
	bucketName      = "data"
	token           = ""
	url             = "http://localhost:8086"
	numPoints       = 3600
	debug           = false
	statName        = "stat"
	blobSize        = 16 * 1024
	numMeasurements = 1
	batchSize       = 1000
	numSeries       = 1
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
		SetBatchSize(uint(batchSize)).
		SetUseGZip(true))
	return influxClient
}

func run(ctx context.Context) error {
	if t := os.Getenv("TOKEN"); t != "" {
		token = t
	}

	flag.StringVar(&url, "url", url, "")
	flag.StringVar(&token, "token", token, "")
	flag.IntVar(&numPoints, "points", numPoints, "")
	flag.IntVar(&blobSize, "blobsize", blobSize, "")
	flag.IntVar(&numMeasurements, "measurements", numMeasurements, "")
	flag.IntVar(&batchSize, "batch", batchSize, "")
	flag.IntVar(&numSeries, "series", numSeries, "")
	flag.Parse()

	// prep for work
	fmt.Println("Preparing for writes...")
	for i := 0; i < numMeasurements; i++ {
		measurement := fmt.Sprintf("stat%d", i)
		err := deleteMeasurement(ctx, bucketName, measurement)
		if err != nil {
			return fmt.Errorf("delete: %w", err)
		}
	}

	// do the actual writing
	fmt.Println("Writing...")
	start := time.Now()
	results := make(chan writeStats, numMeasurements*numSeries)
	for i := 0; i < numMeasurements; i++ {
		measurement := fmt.Sprintf("stat%d", i)
		for t := 0; t < numSeries; t++ {
			tags := map[string]string{"series": fmt.Sprintf("series-%d", t)}
			go func() {
				opts := writeOpts{
					bucket:      bucketName,
					measurement: measurement,
					numPoints:   numPoints,
					tags:        tags,
				}
				res := writeStats{opts: opts}
				start := time.Now()
				res.err = write(ctx, opts)
				res.duration = time.Since(start)
				results <- res
			}()
		}
	}
	totalWrites := 0
	for i := 0; i < numMeasurements*numSeries; i++ {
		res := <-results
		if res.err != nil {
			return res.err
		}
		totalWrites += res.opts.numPoints
		fmt.Printf("%d writes to %s [%v] completed in %v\n",
			res.opts.numPoints, res.opts.measurement, res.opts.tags, res.duration.Truncate(time.Millisecond))
	}
	fmt.Printf("%d writes in %v\n", totalWrites, time.Since(start).Truncate(time.Millisecond))

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
			|> filter(fn: (r) => r._measurement == "`+statName+`")
			|> filter(fn: (r) => r._field == "log")
	`, map[string]string{})
	if err != nil {
		return fmt.Errorf("query: %w", err)
	}
	for res.Next() {
		count++
		record := res.Record()
		if record.Field() == "log" {
			if debug {
				fmt.Printf("> %v\n", record.Value())
			}
		}
	}
	if err = res.Err(); err != nil {
		return fmt.Errorf("res err: %w", err)
	}
	fmt.Printf("Read %d records in %v\n", count, time.Since(start).Truncate(time.Millisecond))
	return nil
}

func deleteMeasurement(ctx context.Context, bucketName string, measurement string) error {
	influxClient := newClient()
	bucket, err := influxClient.BucketsAPI().FindBucketByName(ctx, bucketName)
	if err != nil {
		return fmt.Errorf("find bucket: %w", err)
	}
	orgs, err := influxClient.APIClient().GetOrgs(ctx, &domain.GetOrgsParams{Org: &orgName})
	if err != nil {
		return fmt.Errorf("get orgs: %w", err)
	}
	org := (*orgs.Orgs)[0]
	err = influxClient.DeleteAPI().Delete(ctx, &org, bucket, time.Now().Add(-24*time.Hour), time.Now(), `_measurement="`+measurement+`"`)
	if err != nil {
		return err
	}
	return nil
}

func write(ctx context.Context, opts writeOpts) error {
	influxClient := newClient()
	blob := &bytes.Buffer{}
	for i := 0; i < blobSize; i++ {
		blob.WriteString("x")
	}
	points := make([]*influxWrite.Point, 0, opts.numPoints)
	for i := 0; i < cap(points); i++ {
		max := 1 + rand.Int31n(100)
		avg := rand.Int31n(max)
		ts := time.Now().Add(time.Second * time.Duration(i-cap(points))).Truncate(time.Second)
		tags := map[string]string{
			"unit":   "temperature",
			"client": "unknown",
		}
		for k, v := range opts.tags {
			tags[k] = v
		}
		fields := map[string]any{
			"max":  max,
			"avg":  avg,
			"log":  fmt.Sprintf("Something happened [%d]", i),
			"blob": blob.String(),
		}
		points = append(points, influxdb2.NewPoint(opts.measurement,
			tags,
			fields,
			ts))
	}
	writeAPI := influxClient.WriteAPIBlocking(orgName, opts.bucket)
	writeAPI.EnableBatching()
	err := writeAPI.WritePoint(ctx, points...)
	if err != nil {
		return fmt.Errorf("write points: %w", err)
	}
	err = writeAPI.Flush(ctx)
	if err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}
