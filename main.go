package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
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
	rand.Seed(time.Now().UnixNano())
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func RandStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

var (
	subscribe      = flag.String("subscribe", "", "clash subscribe url, like https://example.com/api/v1/client/subscribe?token=aaaa&flag=clash")
	hiddenPassword = flag.Bool("nopass", false, "hidden password for sharing")
	hiddenBanner   = flag.Bool("nobanner", false, "hidden node emoji banner")
	outFile        = flag.String("c", "config.json", "generated config file path")
	private        = flag.String("private", "", "private domain or domain_suffix list, split by comma")
	clashAPISecret = flag.String("secret", RandStringRunes(8), "clash api secret")
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

var bannerM = map[string]string{
	"香港":    "🇭🇰",
	"日本":    "🇯🇵",
	"美国":    "🇺🇸",
	"新加坡":   "🇸🇬",
	"台湾":    "🇨🇳",
	"英国":    "🇬🇧",
	"法国":    "🇫🇷",
	"冰岛":    "🇮🇸",
	"土耳其":   "🇹🇷",
	"斯洛文尼亚": "🇸🇮",
	"哈萨克斯坦": "🇰🇿",
	"乌克兰":   "🇺🇦",
	"朝鲜":    "🇰🇵",
	"南极":    "🇦🇶",
	"澳门":    "🇲🇴",
	"缅甸":    "🇲🇲",
	"阿根廷":   "🇦🇷",
	"韩国":    "🇰🇷",
}

func groupProxies(ps []map[string]string) map[string][]map[string]string {
	m := make(map[string][]map[string]string)
	for _, p := range ps {
		var k string
		if strings.Contains(p["name"], "香港") {
			k = "香港"
		} else if strings.Contains(p["name"], "日本") {
			k = "日本"
		} else if strings.Contains(p["name"], "美国") {
			k = "美国"
		} else if strings.Contains(p["name"], "新加坡") {
			k = "新加坡"
		} else if strings.Contains(p["name"], "台湾") {
			k = "台湾"
		} else if strings.Contains(p["name"], "英国") {
			k = "英国"
		} else if strings.Contains(p["name"], "法国") {
			k = "法国"
		} else if strings.Contains(p["name"], "冰岛") {
			k = "冰岛"
		} else if strings.Contains(p["name"], "土耳其") {
			k = "土耳其"
		} else if strings.Contains(p["name"], "斯洛文尼亚") {
			k = "斯洛文尼亚"
		} else if strings.Contains(p["name"], "哈萨克斯坦") {
			k = "哈萨克斯坦"
		} else if strings.Contains(p["name"], "乌克兰") {
			k = "乌克兰"
		} else if strings.Contains(p["name"], "朝鲜") {
			k = "朝鲜"
		} else if strings.Contains(p["name"], "南极") {
			k = "南极"
		} else if strings.Contains(p["name"], "澳门") {
			k = "澳门"
		} else if strings.Contains(p["name"], "缅甸") {
			k = "缅甸"
		} else if strings.Contains(p["name"], "阿根廷") {
			k = "阿根廷"
		} else if strings.Contains(p["name"], "韩国") {
			k = "韩国"
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

type DialField struct {
	TCPMultiPath bool `json:"tcp_multi_path"`
}

type Shadowsocks struct {
	Type        string `json:"type"`
	Tag         string `json:"tag"`
	Server      string `json:"server"`
	ServerPort  int    `json:"server_port"`
	Method      string `json:"method"`
	Password    string `json:"password"`
	RoutingMark int    `json:"routing_mark,omitempty"`
	*DialField
}

type Vmess struct {
	Type        string `json:"type"`
	Tag         string `json:"tag"`
	Server      string `json:"server"`
	ServerPort  int    `json:"server_port"`
	UUID        string `json:"uuid"`
	AlterID     int    `json:"alter_id"`
	Security    string `json:"security"`
	RoutingMark int    `json:"routing_mark,omitempty"`
	*DialField
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
	Outbounds []string `json:"outbounds"`
	Default   string   `json:"default"`
}

type Direct struct {
	Type        string `json:"type"`
	Tag         string `json:"tag"`
	RoutingMark int    `json:"routing_mark,omitempty"`
}

type Block struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type DNS struct {
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type CustomOutbounds struct {
	Outbounds    []interface{}
	DNSHosts     []string
	GeositeItems []string
}

func generateOutbounds(gp map[string][]map[string]string, hiddenPassword bool, hiddenBanner bool) *CustomOutbounds {
	var ms []interface{}
	var allItems []string
	var allRegions []string
	var allHosts []string
	for k, v := range gp {
		if !hiddenBanner {
			k = fmt.Sprintf("%s %s", bannerM[k], k)
		}
		var item []string
		for i, p := range v {
			var m interface{}
			tag := fmt.Sprintf("%s %02d", k, i+1)
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
					DialField:  &DialField{TCPMultiPath: true},
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
					DialField:  &DialField{TCPMultiPath: true},
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
		ms = append(ms, &URLTest{
			Type:      "urltest",
			Tag:       k,
			URL:       testURL,
			Interval:  "1m",
			Tolerance: 50,
			Outbounds: item,
		})
	}

	// auto
	ms = append(ms, &URLTest{
		Type:      "urltest",
		Tag:       "♻ 自动选择",
		URL:       testURL,
		Interval:  "1m",
		Tolerance: 5000,
		Outbounds: allItems,
	})

	// select
	items := append([]string{"♻ 自动选择"})
	for _, v := range allRegions {
		if !strings.Contains(v, "kp") {
			items = append(items, v)
		}
	}
	items = append(items, allItems...)
	ms = append(ms, &Selector{
		Type:      "selector",
		Tag:       "🚀 手动选择",
		Outbounds: items,
		Default:   "♻ 自动选择",
	})

	// custom geosite selectors
	var customGeositeItems []string
	ms = append(ms, &Selector{
		Type:      "selector",
		Tag:       "🤖 人工智能",
		Outbounds: append([]string{"🚀 手动选择"}, allRegions...),
		Default:   "🚀 手动选择",
	})
	customGeositeItems = append(customGeositeItems, "🤖 人工智能")
	ms = append(ms, &Selector{
		Type:      "selector",
		Tag:       "🎶 声破天",
		Outbounds: append([]string{"direct-out", "🚀 手动选择"}, allItems...),
		Default:   "direct-out",
	})
	customGeositeItems = append(customGeositeItems, "🎶 声破天")
	ms = append(ms, &Selector{
		Type:      "selector",
		Tag:       "🎥 奈飞",
		Outbounds: append([]string{"🚀 手动选择"}, allItems...),
		Default:   "🚀 手动选择",
	})
	customGeositeItems = append(customGeositeItems, "🎥 奈飞")

	// needed
	ms = append(ms, &Direct{
		Type: "direct",
		Tag:  "direct-out",
	})
	ms = append(ms, &Block{
		Type: "block",
		Tag:  "block-out",
	})
	ms = append(ms, &DNS{
		Type: "dns",
		Tag:  "dns-out",
	})

	return &CustomOutbounds{
		Outbounds:    ms,
		DNSHosts:     allHosts,
		GeositeItems: customGeositeItems,
	}
}

//go:embed static/default.json
var config []byte

type DNSRule struct {
	Domain       []string `json:"domain"`
	DomainSuffix []string `json:"domain_suffix"`
	Servers      string   `json:"server"`
}

type Rule struct {
	Geosite      []string `json:"geosite,omitempty"`
	Outbound     string   `json:"outbound"`
	Domain       []string `json:"domain,omitempty"`
	DomainSuffix []string `json:"domain_suffix,omitempty"`
}

type Geo struct {
	Path           string `json:"path,omitempty"`
	DownloadURL    string `json:"download_url"`
	DownloadDetour string `json:"download_detour"`
}

type Route struct {
	Geoip               *Geo          `json:"geoip"`
	Geosite             *Geo          `json:"geosite"`
	Rules               []interface{} `json:"rules"`
	Final               string        `json:"final"`
	AutoDetectInterface bool          `json:"auto_detect_interface"`
	OverrideAndroidVPN  bool          `json:"override_android_vpn"`
}

type ClashAPI struct {
	ExternalController    string          `json:"external_controller"`
	ExternalUI            json.RawMessage `json:"external_ui"`
	ExternalUIDownloadURL string          `json:"external_ui_download_url"`
	StoreSelected         bool            `json:"store_selected"`
	Secret                string          `json:"secret"`
}

type Config struct {
	Log json.RawMessage `json:"log"`
	DNS struct {
		Servers  json.RawMessage `json:"servers"`
		Rules    []interface{}   `json:"rules"`
		Strategy string          `json:"strategy"`
	} `json:"dns"`
	Outbounds    interface{}     `json:"outbounds"`
	Inbounds     json.RawMessage `json:"inbounds"`
	Route        Route           `json:"route"`
	Experimental struct {
		ClashAPI ClashAPI `json:"clash_api"`
	} `json:"experimental"`
}

var geositeM = map[string]string{
	"🤖 人工智能": "openai",
	"🎶 声破天":  "spotify",
	"🎥 奈飞":   "netflix",
}

func generateConfig(out *CustomOutbounds, privateDomains string, clashAPISecret string, configPath string) error {
	var cfg Config
	if err := json.Unmarshal(config, &cfg); err != nil {
		return err
	}

	// subscribe hosts to dns direct
	cfg.DNS.Rules = append([]interface{}{&DNSRule{
		Domain:  out.DNSHosts,
		Servers: "ali",
	}}, cfg.DNS.Rules...)

	// added custom geosite items
	rules := make([]interface{}, 0, len(cfg.Route.Rules)+len(out.GeositeItems))
	rules = append(rules, cfg.Route.Rules[:2]...)
	for _, v := range out.GeositeItems {
		rules = append(rules, &Rule{
			Geosite:  []string{geositeM[v]},
			Outbound: v,
		})
	}
	// private domains
	if privateDomains != "" {
		r := &Rule{}
		ds := strings.Split(privateDomains, ",")
		for _, v := range ds {
			if strings.HasPrefix(v, ".") {
				r.DomainSuffix = append(r.DomainSuffix, v)
			} else {
				r.Domain = append(r.Domain, v)
			}
		}
		if len(r.Domain) > 0 || len(r.DomainSuffix) > 0 {
			r.Outbound = "direct-out"
			rules = append(rules, r)

			// dns rules
			cfg.DNS.Rules = append([]interface{}{&DNSRule{
				Domain:       r.Domain,
				DomainSuffix: r.DomainSuffix,
				Servers:      "ali",
			}}, cfg.DNS.Rules...)
		}
	}

	// nodes
	rules = append(rules, &Rule{
		Domain:   out.DNSHosts,
		Outbound: "direct-out",
	})

	rules = append(rules, cfg.Route.Rules[2:]...)
	cfg.Route.Rules = rules

	// clash api Secret
	cfg.Experimental.ClashAPI.Secret = clashAPISecret

	cfg.Outbounds = out.Outbounds

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

	ob := generateOutbounds(groupProxies(ps), *hiddenPassword, *hiddenBanner)
	if err = generateConfig(ob, *private, *clashAPISecret, *outFile); err != nil {
		panic(err)
	}
}
