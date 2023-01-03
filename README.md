# influx-playground

To start:

	dc up -d

This will start influxdb running on port 8086.

To start using it, you'll need to first create an API token:

	export TOKEN=$(dc exec influxdb influx auth create --operator --json | jq -r .token | xargs echo -n)

# Writing data

The `./cmd/write` package tests the performance of concurrent writes to influxdb. 

## Defaults

Measurement:
- `stat0`

Tags:
- `unit`: `temperature`
- `client`: `unknown`
- `series`: `series-0`

Fields:
- `max`: random number
- `avg`: random number
- `log`: unique log message
- `blob`: arbitrary string data (16KB)

## Flags

You can control how many datapoints are written per series by specifying `--points 3600` (to create an hour's worth of points)

You can create multiple series by using `--tags N`. This will result in separate series using the `series` tag with incrementing values.

You can also create multiple measurements using `--measurements N`. This creates new "metrics" as opposed to create separate series within the same metric using different tag sets.

You can change the size of the `blob` payload by specifying `--blobsize 4096` (to create 4K payloads instead of 16K payloads)

## Example

	go run ./cmd/write --points 3600 --blobsize 8096 --series 8	


