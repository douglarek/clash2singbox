package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sagernet/sing-box/option"
	"gopkg.in/yaml.v3"
)

func init() {
	http.DefaultClient.Timeout = 10 * time.Second
}

var (
	subscribe      = flag.String("subscribe", "", "clash subscribe url, like https://example.com/api/v1/client/subscribe?token=aaaa&flag=clash")
	hiddenPassword = flag.Bool("nopass", false, "hidden password for sharing")
	outFile        = flag.String("c", "config.json", "generated config file path")
)

const (
	agent   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36"
	testURL = "https://www.gstatic.com/generate_204"
)

func parseSubscribeProxies(url string) ([]map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Close = true
	req.Header.Set("User-Agent", agent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: %s", resp.Status)
	}

	var s struct {
		Proxies []map[string]string `yaml:"proxies"`
	}

	if err = yaml.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}

	return s.Proxies, nil
}

func groupProxies(ps []map[string]string) map[string][]map[string]string {
	m := make(map[string][]map[string]string)
	for _, p := range ps {
		var k string
		if strings.Contains(p["name"], "香港") {
			k = "hk"
		} else if strings.Contains(p["name"], "日本") {
			k = "jp"
		} else if strings.Contains(p["name"], "美国") {
			k = "us"
		} else if strings.Contains(p["name"], "新加坡") {
			k = "sg"
		} else if strings.Contains(p["name"], "台湾") {
			k = "tw"
		}

		if k == "" {
			continue
		}

		if _, ok := m[k]; !ok {
			m[k] = make([]map[string]string, 0)
		}
		m[k] = append(m[k], p)
	}
	return m
}

type Shadowsocks struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Method     string `json:"method"`
	Password   string `json:"password"`
}

type Vmess struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	UUID       string `json:"uuid"`
	AlterID    int    `json:"alter_id"`
	Security   string `json:"security"`
}

type URLTest struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	URL       string   `json:"url"`
	Interval  string   `json:"interval"`
	Tolerance int      `json:"tolerance"`
	Outbounds []string `json:"outbounds"`
}

type Selector struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	Outbounds []string `json:"outbounds,omitempty"`
	Default   string   `json:"default,omitempty"`
}

func generateOutbounds(gp map[string][]map[string]string, hiddenPassword bool) ([]interface{}, []string) {
	var ms []interface{}
	var allItems []string
	var allRegions []string
	var allHosts []string
	for k, v := range gp {
		var item []string
		for i, p := range v {
			var m interface{}
			tag := fmt.Sprintf("%s-%02d", k, i+1)
			port, err := strconv.Atoi(p["port"])
			if err != nil {
				panic(err)
			}
			switch p["type"] {
			case "ss":
				m = &Shadowsocks{
					Type:       "shadowsocks",
					Tag:        tag,
					Server:     p["server"],
					ServerPort: port,
					Method:     p["cipher"],
				}
				if hiddenPassword {
					m.(*Shadowsocks).Password = "******"
				} else {
					m.(*Shadowsocks).Password = p["password"]
				}
			case "vmess":
				m = &Vmess{
					Type:       "vmess",
					Tag:        tag,
					Server:     p["server"],
					ServerPort: port,
				}
				if hiddenPassword {
					m.(*Vmess).UUID = "******"
				} else {
					m.(*Vmess).UUID = p["uuid"]
				}
				aid, err := strconv.Atoi(p["alterId"])
				if err != nil {
					panic(err)
				}
				m.(*Vmess).AlterID = aid
				m.(*Vmess).Security = p["cipher"]
			default:
				panic(fmt.Errorf("unknown type: %s", p["type"]))
			}
			ms = append(ms, m)
			item = append(item, tag)
			allItems = append(allItems, tag)
			allHosts = append(allHosts, p["server"])
		}

		allRegions = append(allRegions, k)

		// regions
		m := &URLTest{
			Type:      "urltest",
			Tag:       k,
			URL:       testURL,
			Interval:  "1m",
			Tolerance: 50,
			Outbounds: item,
		}
		ms = append(ms, m)
	}

	// auto
	var m interface{} = &URLTest{
		Type:      "urltest",
		Tag:       "auto",
		URL:       testURL,
		Interval:  "1m",
		Tolerance: 50,
		Outbounds: allItems,
	}
	ms = append(ms, m)

	// select
	m = &Selector{
		Type:    "selector",
		Tag:     "select",
		Default: "auto",
	}
	items := []string{"auto"}
	items = append(items, allRegions...)
	items = append(items, allItems...)
	m.(*Selector).Outbounds = items
	ms = append(ms, m)

	m = &Selector{
		Type:    "selector",
		Tag:     "netflix",
		Default: "select",
	}
	items = []string{"select"}
	items = append(items, allItems...)
	m.(*Selector).Outbounds = items
	ms = append(ms, m)

	m = &Selector{
		Type:    "selector",
		Tag:     "spotify",
		Default: "direct",
	}
	items = []string{"direct", "select"}
	items = append(items, allItems...)
	m.(*Selector).Outbounds = items
	ms = append(ms, m)

	m = &Selector{
		Type: "direct",
		Tag:  "direct",
	}
	ms = append(ms, m)

	m = &Selector{
		Type: "block",
		Tag:  "block",
	}
	ms = append(ms, m)

	m = &Selector{
		Type: "dns",
		Tag:  "dns-out",
	}
	ms = append(ms, m)
	return ms, allHosts
}

//go:embed tmpl.json
var config []byte

type Config struct {
	Log json.RawMessage `json:"log"`
	DNS struct {
		Servers  json.RawMessage          `json:"servers"`
		Rules    []map[string]interface{} `json:"rules"`
		Strategy string                   `json:"strategy"`
	} `json:"dns"`
	Outbounds    interface{}     `json:"outbounds"`
	Inbounds     json.RawMessage `json:"inbounds"`
	Route        json.RawMessage `json:"route"`
	Experimental json.RawMessage `json:"experimental"`
}

func generateConfig(outbounds []interface{}, allHosts []string, configPath string) error {
	var cfg Config
	if err := json.Unmarshal(config, &cfg); err != nil {
		return err
	}

	// subscribe hosts to dns direct
	m := make(map[string]interface{})
	m["domain"] = allHosts
	m["geosite"] = "cn"
	m["server"] = "local"
	cfg.DNS.Rules = append(cfg.DNS.Rules, m)

	cfg.Outbounds = outbounds

	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}

	return format(configPath, b)
}

// format func modified from https://github.com/SagerNet/sing-box/blob/dev-next/cmd/sing-box/cmd_format.go
func format(configPath string, content []byte) error {
	var options option.Options
	err := options.UnmarshalJSON(content)
	if err != nil {
		return err
	}
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(options)
	if err != nil {
		return err
	}
	if bytes.Equal(content, buffer.Bytes()) {
		return nil
	}
	output, err := os.Create(configPath)
	if err != nil {
		return err
	}
	_, err = output.Write(buffer.Bytes())
	output.Close()
	if err != nil {
		return err
	}
	outputPath, _ := filepath.Abs(configPath)
	os.Stderr.WriteString(outputPath + "\n")
	return nil
}

func main() {
	flag.Parse()

	if *subscribe == "" {
		flag.Usage()
		return
	}

	ps, err := parseSubscribeProxies(*subscribe)
	if err != nil {
		panic(err)
	}

	ob, addrs := generateOutbounds(groupProxies(ps), *hiddenPassword)
	if err = generateConfig(ob, addrs, *outFile); err != nil {
		panic(err)
	}
}
