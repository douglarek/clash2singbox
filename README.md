# clash2singbox

A tool for converting Clash subscriptions to Sing-Box configurations.

## Install

```
$ go install github.com/douglarek/clash2singbox@main
```

## Usage

```
$ clash2singbox --help

Usage of clash2singbox:
  -c string
        generated config file path (default "config.json")
  -mode string
        general or hp. general is for general purpose, hp is for Immortalwrt homeproxy (default "general")
  -nobanner
        hidden node emoji banner
  -nopass
        hidden password for sharing
  -private string
        private domain or domain_suffix list, split by comma
  -secret string
        clash api secret (default "dZZBlNUD")
  -subscribe string
        clash subscribe url, like https://example.com/api/v1/client/subscribe?token=aaaa&flag=clash
```

## Dashboard

Access [dashboard](https://yacd.metacubex.one/) from your browser:

![Clash dashboard](https://github.com/douglarek/clash2singbox/raw/main/static/dashboard.png)
