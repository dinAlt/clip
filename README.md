# Clip
Tool for clipping content of web pages to PDF files.

## Overfiew
**Clip** can be used as CLI, REST service, AWS Lambda function or Go library.

## Prerequisites
- working [Go](https://golang.org/) installation (version 1.15 or higher).
- [wkhtmltopdf](https://wkhtmltopdf.org/) need to be in your `PATH` environment variable. You can also set `WKHTMLTOPDF_PATH` to target wkhtmlotpdf directory, or just place executable in `clip`'s directory (see [go-wkhtmltopdf](https://github.com/SebastiaanKlippert/go-wkhtmltopdf#installation) reference)

## Installation
CLI:
```shell
go install github.com/dinalt/clip/cmd/clip
```
REST service:
```shell
go install github.com/dinalt/clip/cmd/clip-serve
```

## Usage examples
### CLI
Use `clip -h` to get list of available format arguments.

Clip article from site habr.com using [presets](#presets):
```shell
clip -presets-path ./presets.json -p auto,margins:a4 https://habr.com/en/post/510746/ habr.pdf
```
Clip index article from site restfulapi.net:
```shell
 clip -query .content -remove .comment-respond -custom-styles .content{width:auto} https://restfulapi.net/ rest.pdf

```

### REST service
Use `clip-serve -h` to get REST service launch arguments list.

Launch service:
```shell
clip-serve -a :8080 -p ./presets.json -w 5
```
Clip article from site habr.com using [presets](#presets):
```shell
curl http://localhost:8080/v0/clip\?presets\=auto,margins:a4\&url\=https://habr.com/ru/post/263897/ --output habr.pdf

```
POST queries are also allowed via form params or json object (with `Content-Type: application/json` provided).

## Presets
Presets are useful shortcuts for common used parameters sets. Definition samples can be found in file `presets.json` in root of this repository.

To use custom preset (or list of presets), provide `-p` param (for CLI) or `presets` parameter (for REST query), with comma-separated preset names: `presets=habr:post,margins:a4`.

Presets are applied in the specified order and do not overwrite settings already set. Settings qualified in query or CLI params have **higher** priority.

### Auto
`auto` is a special preset, which tells `clip` (CLI or REST service or Lambda function) to infer preset from site's url, using `url_regexp` field of preset JSON object (see example in `presets.json`)

## Build for AWS Lambda
```shell
git clone https://github.com/dinAlt/clip
cd clip
CGO_ENABLED=0 go build -o clip-lambda cmd/clip-lambda/main.go
```
## More info
- All sizes (margins, page width and height) are **integer** values (in **millimeters**).
- `query`, `remove`, `break_before`, `no_break_inside` and `no_break_after` values should be a valid [css selectors](http://butlerccwebdev.net/support/css-selectors-cheatsheet.html).
- Javascript is disabled for page rendering by default, but you can enable it via setting `enable_javascript` param value to `true`.
- Use `custom_styles` parameter to adjust result PDF document view.
- `query` and `remove` parameters doesn't work for progressive web apps (`PWA`), because they are modify DOM before javascript executed. Try to use `custom_styles`, if this is your case.

## Supported OS
- Tested on `Linux`.
- On `MacOS` also should work fine (but not tested).
- On `Windows` may work with `wkhtmltopdf` added to `PATH`. In other cases shouldn't work due hardcoded executable name (without `.exe` extension).

## Contribution
Pull requests and issues (especially for adding new presets) are welcome!
