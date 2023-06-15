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

func parseSubscribeProxies(url string) ([]map[string]string, error) {
	resp, err := http.Get(url)
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

func generateOutbounds(gp map[string][]map[string]string, hiddenPassword bool) ([]map[string]interface{}, []string) {
	var ms []map[string]interface{}
	var allItems []string
	var allRegions []string
	var allHosts []string
	for k, v := range gp {
		var item []string
		for i, p := range v {
			m := make(map[string]interface{})
			m["tag"] = fmt.Sprintf("%s-%02d", k, i+1)
			port, err := strconv.Atoi(p["port"])
			if err != nil {
				panic(err)
			}
			switch p["type"] {
			case "ss":
				m["type"] = "shadowsocks"
				m["server"] = p["server"]
				m["server_port"] = port
				m["method"] = p["cipher"]
				if hiddenPassword {
					m["password"] = "******"
				} else {
					m["password"] = p["password"]
				}
			case "vmess":
				m["type"] = "vmess"
				m["server"] = p["server"]
				m["server_port"] = port
				m["uuid"] = p["uuid"]
				aid, err := strconv.Atoi(p["alterId"])
				if err != nil {
					panic(err)
				}
				m["alter_id"] = aid
				m["security"] = p["cipher"]
			default:
				panic(fmt.Errorf("unknown type: %s", p["type"]))
			}
			ms = append(ms, m)
			item = append(item, m["tag"].(string))
			allItems = append(allItems, m["tag"].(string))
			allHosts = append(allHosts, p["server"])
		}

		allRegions = append(allRegions, k)

		// regions
		m := make(map[string]interface{})
		m["type"] = "urltest"
		m["tag"] = k
		m["url"] = "https://www.gstatic.com/generate_204"
		m["interval"] = "1m"
		m["tolerance"] = 50
		m["outbounds"] = item
		ms = append(ms, m)
	}

	// auto
	m := make(map[string]interface{})
	m["type"] = "urltest"
	m["tag"] = "auto"
	m["url"] = "https://www.gstatic.com/generate_204"
	m["interval"] = "1m"
	m["tolerance"] = 50
	m["outbounds"] = allItems
	ms = append(ms, m)

	// select
	m = make(map[string]interface{})
	m["type"] = "selector"
	m["tag"] = "select"
	items := []string{"auto"}
	items = append(items, allRegions...)
	items = append(items, allItems...)
	m["outbounds"] = items
	m["default"] = "auto"
	ms = append(ms, m)

	m = make(map[string]interface{})
	m["type"] = "selector"
	m["tag"] = "netflix"
	m["outbounds"] = append([]string{"select"}, allItems...)
	m["default"] = "select"
	ms = append(ms, m)

	// ms
	m = make(map[string]interface{})
	m["type"] = "selector"
	m["tag"] = "microsoft"
	m["outbounds"] = append([]string{"direct", "select"}, allItems...)
	m["default"] = "direct"
	ms = append(ms, m)

	// direct
	m = make(map[string]interface{})
	m["type"] = "direct"
	m["tag"] = "direct"
	ms = append(ms, m)

	// block
	m = make(map[string]interface{})
	m["type"] = "block"
	m["tag"] = "block"
	ms = append(ms, m)

	// dns
	m = make(map[string]interface{})
	m["type"] = "dns"
	m["tag"] = "dns-out"
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

func generateConfig(outbounds []map[string]interface{}, allHosts []string, configPath string) error {
	var cfg Config
	if err := json.Unmarshal(config, &cfg); err != nil {
		return err
	}
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
