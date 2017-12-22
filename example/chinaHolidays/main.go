// Package main 根据双休日或者单休日生成节假日，然后从国家放假安排的每个节日的json格式修正数据，从而导出最终的时间表
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/qingtao/holiday"
)

// WhiteList 用于设置http服务器的IP地址白名单
type WhiteList struct {
	Lock   sync.Mutex
	IPs    map[string]net.IP
	IPNets map[string]*net.IPNet
}

// Server 向http.ServeMux添加白名单功能
type Server struct {
	*http.ServeMux
	Logger *log.Logger

	WhiteList *WhiteList
	exit      chan struct{}
}

type Message struct {
	Status string      `json:"status"`
	Msg    interface{} `json:"msg"`
}

// Verify 检查ip权限
func (wl *WhiteList) Verify(ip net.IP) bool {
	for _, ipnet := range wl.IPNets {
		if ipnet.Contains(ip) {
			return true
		}
	}
	for _, ip1 := range wl.IPs {
		if ip1.Equal(ip) {
			return true
		}
	}
	return false
}

// Update 更新白名单，action可以是ADD/UPDATE和DEL
func (wl *WhiteList) Update(action, s string) error {
	wl.Lock.Lock()
	defer wl.Lock.Unlock()
	ips, ipnets := make(map[string]net.IP), make(map[string]*net.IPNet)
	// 查找白名单的每一行，并忽略#号开头的注释内容
	for _, line := range strings.Fields(s) {
		if strings.HasPrefix(line, "#") {
			continue
		}
		// 解析CIDR格式的IP网段
		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			// 若解析网段错误按照IP地址解析
			allowIP := net.ParseIP(line)
			if allowIP == nil {
				return fmt.Errorf("not a valid ip address: %s\n", err)
			}
			ips[line] = allowIP
			continue
		}
		ipnets[line] = ipNet
	}

	switch action {
	// 目前ADD和UPDATE的操作相同
	case "ADD", "UPDATE":
	TOPL:
		for k, v := range ipnets {
			for wk, wv := range wl.IPNets {
				// 判断白名单当前网段是否包含目标IP，或者新白名单网段是否包含当前已存在网段
				if wv.Contains(v.IP) || v.Contains(wv.IP) {
					wOnes, _ := wv.Mask.Size()
					ones, _ := v.Mask.Size()
					// 若新的网段掩码长度小于旧的网段，删除旧的网段并添加wl.IPNets[k] = v
					if ones < wOnes {
						delete(wl.IPNets, wk)
						wl.IPNets[k] = v
					}
					//跳到第一个循环开始
					continue TOPL
				}
			}
			wl.IPNets[k] = v
		}
		// 逐条添加ip地址到白名单
		for k, v := range ips {
			if wl.Verify(v) {
				continue
			}
			wl.IPs[k] = v
		}
	case "DEL":
		for k, _ := range ipnets {
			delete(wl.IPNets, k)
		}
		for k, _ := range ips {
			delete(wl.IPs, k)
		}
	}
	return nil
}

// NewServer 读取白名单: 每一行一个IP或者网段
func NewServer(whitelist string, l *log.Logger) (*Server, error) {
	mux := http.NewServeMux()
	b, err := ioutil.ReadFile(whitelist)
	if err != nil {
		return nil, err
	}

	var s = new(Server)
	s.ServeMux = mux

	wl := &WhiteList{
		IPs:    make(map[string]net.IP),
		IPNets: make(map[string]*net.IPNet),
	}
	if err = wl.Update("ADD", string(b)); err != nil {
		return nil, err
	}
	s.WhiteList = wl
	s.Logger = l
	s.exit = make(chan struct{})
	return s, nil
}

func VerifyClient(r *http.Request, wl *WhiteList) error {
	var ip string
	// 此服务可能运行在nginx/apache后
	if xforwarfor := r.Header.Get("X-Forward-For"); xforwarfor != "" {
		if wl.Verify(net.ParseIP(xforwarfor)) {
			return nil
		}
		ip = xforwarfor
	} else if userIp, _, err := net.SplitHostPort(r.RemoteAddr); err != nil {
		return fmt.Errorf("client ip: %s is not IP:port", userIp)
	} else {
		ip = userIp
		if wl.Verify(net.ParseIP(ip)) {
			return nil
		}
	}
	// 客户端IP地址不在白名单之内，拒绝请求
	return fmt.Errorf("client ip: %s is not allowed", ip)
}

func (s *Server) VerifyClient(w http.ResponseWriter, r *http.Request) bool {
	if err := VerifyClient(r, s.WhiteList); err != nil {
		s.Logger.Printf("%s\n", err)
		msg, err := GenMessage("failed", err.Error())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return false
		}
		w.Header().Set("Content-Type", "text/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", msg)
		return false
	}
	return true
}

/*
// SerHTTP 实现ServeHTTP: 并检查客户端IP地址是否允许访问
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//全局白名单检查, 可以考虑放置VerifyClient到每个需要检查客户端的http.HandleFunc
	//srv.VerifyClient(w, r)
	s.ServeMux.ServeHTTP(w, r)
}
*/

func GenMessage(s string, a interface{}) ([]byte, error) {
	msg := Message{s, a}
	b, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ServeHolidays 返回节假日JSON给客户端
func ServeHolidays(w http.ResponseWriter, r *http.Request, h map[int]*holiday.HolidaysOfYear) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	year := r.FormValue("year")
	if year == "" {
		msg, err := GenMessage("failed", "year is empty")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", msg)
		return
	}
	year = strings.ToLower(year)
	y, err := strconv.Atoi(year)
	if err != nil {
		msg, err := GenMessage("failed", "year is not a integer")
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", msg)
		return
	}

	hy, ok := h[y]
	if !ok {
		msg, err := GenMessage("failed", fmt.Sprintf("holidays of %d not exists", y))
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "%s", msg)
		return
	}

	var i interface{} = hy

	month := r.FormValue("month")
	if month != "" {
		m, err := strconv.Atoi(month)
		var st string
		if err != nil {
			st = "month is not integer"
		} else if m < 1 || m > 12 {
			st = "month must between 1 and 12"
		} else {
			hm, ok := hy.Month[m]
			if !ok {
				st = fmt.Sprintf("holidays of %d%d is not exists", y, m)
			}
			i = hm
		}
		if st != "" {
			msg, err := GenMessage("failed", st)
			if err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "text/json; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "%s", msg)
			return
		}
	}
	msg, err := GenMessage("success", i)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	fmt.Fprintf(w, "%s", msg)
}

func ServeMain(w http.ResponseWriter, r *http.Request, s string) {
	w.Header().Set("Content-Type", "text/json; charset=utf-8")
	fmt.Fprint(w, s)
}

func main() {
	file := "whitelist.cnf"
	l := log.New(os.Stdout, "", 1|2)
	s, err := NewServer(file, l)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Printf("%+v\n", s.WhiteList)
	hs := map[int]*holiday.HolidaysOfYear{
		2018: holiday.NewHolidaysOfYear(2018, 0, 6),
	}

	fs, err := filepath.Glob("2018/2018*.json")
	if err != nil {
		log.Fatalln(err)
	}
	for i := 0; i < len(fs); i++ {
		b, err := ioutil.ReadFile(fs[i])
		if err != nil {
			log.Fatalln(err)
		}
		var chd holiday.ChineseHoliday
		if err := json.Unmarshal(b, &chd); err != nil {
			log.Fatalln(err)
		}
		hs[2018].Update(&chd)
	}

	s.HandleFunc("/hs", func(w http.ResponseWriter, r *http.Request) {
		if !s.VerifyClient(w, r) {
			return
		}
		ServeHolidays(w, r, hs)
	})
	s.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ServeMain(w, r, "hello!")
	})
	addr := ":10082"
	http.ListenAndServe(addr, s)
}
