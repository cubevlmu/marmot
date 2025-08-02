package onebot

import (
	"github.com/goccy/go-json"
	"marmot/utils"

	"github.com/RomiChan/websocket"
	"github.com/tidwall/gjson"

	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

var (
	nullResponse = APIResponse{}
)

// WSServer ...
type WSServer struct {
	URL         string
	AccessToken string
	lstn        net.Listener
	caller      chan *WSSCaller
	hook        ConnectHook

	json.Unmarshaler
}

// UnmarshalJSON init WSServer with waitn=16
func (wss *WSServer) UnmarshalJSON(data []byte) error {
	type jsoncfg struct {
		URL         string // ws连接地址
		AccessToken string
	}
	err := json.Unmarshal(data, (*jsoncfg)(unsafe.Pointer(wss)))
	if err != nil {
		return err
	}
	wss.caller = make(chan *WSSCaller, 16)
	return nil
}

// hack

type ConnectHook func(id int64)

// NewWebSocketServer 使用反向WS通信
func NewWebSocketServer(waitn int, url, accessToken string, hook ConnectHook) *WSServer {
	return &WSServer{
		URL:         url,
		AccessToken: accessToken,
		caller:      make(chan *WSSCaller, waitn),
		hook:        hook,
	}
}

// WSSCaller ...
type WSSCaller struct {
	mu     sync.Mutex // 写锁
	seqMap SeqSyncMap
	conn   *websocket.Conn
	selfID int64
	seq    uint64
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool {
		return true
	},
}

func (wss *WSServer) Connect() {
	network, address := utils.ResolveURI(wss.URL)
	uri, err := url.Parse(address)
	if err == nil && uri.Scheme != "" {
		address = uri.Host
	}

	listener, err := net.Listen(network, address)
	if err != nil {
		LogWarn("[wss] failed to listen at (WS_Server): %v", err)
		wss.lstn = nil
		return
	}

	wss.lstn = listener
	LogInfo("[wss] websocket server listening at port: %s", listener.Addr())
}

func checkAuth(req *http.Request, token string) int {
	if token == "" { // quick path
		return http.StatusOK
	}

	auth := req.Header.Get("Authorization")
	if auth == "" {
		auth = req.URL.Query().Get("access_token")
	} else {
		_, after, ok := strings.Cut(auth, " ")
		if ok {
			auth = after
		}
	}

	switch auth {
	case token:
		return http.StatusOK
	case "":
		return http.StatusUnauthorized
	default:
		return http.StatusForbidden
	}
}

func (wss *WSServer) any(w http.ResponseWriter, r *http.Request) {
	status := checkAuth(r, wss.AccessToken)
	if status != http.StatusOK {
		LogWarn("[wss] refused websocket connection of %v : invalid token (code:%d)", r.RemoteAddr, status)
		w.WriteHeader(status)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		LogWarn("[wss] error occured when handling webSocket request: %v", err)
		return
	}

	var rsp struct {
		SelfID int64 `json:"self_id"`
	}
	err = conn.ReadJSON(&rsp)
	if err != nil {
		LogWarn("[wss] handshake with websocket server %v failed: %v", wss.URL, err)
		return
	}

	c := &WSSCaller{
		conn:   conn,
		selfID: rsp.SelfID,
	}
	APICallers.Store(rsp.SelfID, c) // add Caller to APICaller list...
	if wss.hook != nil {
		wss.hook(rsp.SelfID)
	}
	LogInfo("[wss] connected to websocket server: %s QQ account : %d", wss.URL, rsp.SelfID)
	wss.caller <- c
}

func (wss *WSServer) Listen(handler func([]byte, APICaller)) {
	mux := http.ServeMux{}
	mux.HandleFunc("/", wss.any)
	go func() {
		for {
			if wss.lstn == nil {
				time.Sleep(time.Millisecond * time.Duration(3))
				wss.Connect()
				continue
			}
			LogInfo("[wss] webSocket server handling : %v", wss.lstn.Addr())
			err := http.Serve(wss.lstn, &mux)
			if err != nil {
				LogWarn("[wss] websocket server occured an error at end point : %s with error : %v", wss.lstn.Addr(), err)
				wss.lstn = nil
			}
		}
	}()
	for wssc := range wss.caller {
		go wssc.listen(handler)
	}
}

func (wssc *WSSCaller) listen(handler func([]byte, APICaller)) {
	for {
		t, payload, err := wssc.conn.ReadMessage()
		if err != nil { // reconnect
			APICallers.Delete(wssc.selfID) // remove from caller map when disconnect
			LogWarn("[wss] disconnected from websocket server, QQ account : %v", wssc.selfID)
			return
		}
		if t != websocket.TextMessage {
			continue
		}
		rsp := gjson.Parse(utils.BytesToString(payload))
		if rsp.Get("echo").Exists() { // api reponse (echo field)
			LogDebug("[wss] received from api calling : %v", strings.TrimSpace(utils.BytesToString(payload)))
			if c, ok := wssc.seqMap.LoadAndDelete(rsp.Get("echo").Uint()); ok {
				msg := rsp.Get("message").Str
				if msg == "" {
					msg = rsp.Get("msg").Str
				}
				c <- APIResponse{ // send api and wait response
					Status:  rsp.Get("status").String(),
					Data:    rsp.Get("data"),
					Message: msg,
					Wording: rsp.Get("wording").Str,
					RetCode: rsp.Get("retcode").Int(),
					Echo:    rsp.Get("echo").Uint(),
				}
				close(c) // channel only use once
			}
			continue
		}
		if rsp.Get("meta_event_type").Str == "heartbeat" { // ignore heartbeat packet
			continue
		}
		LogDebug("[wss] received event : %v", utils.BytesToString(payload))
		handler(payload, wssc)
	}
}

func (wssc *WSSCaller) nextSeq() uint64 {
	return atomic.AddUint64(&wssc.seq, 1)
}

func (wssc *WSSCaller) CallAPI(req APIRequest) (APIResponse, error) {
	ch := make(chan APIResponse, 1)
	req.Echo = wssc.nextSeq()
	wssc.seqMap.Store(req.Echo, ch)

	// send message
	wssc.mu.Lock() // websocket write is not goroutine safe
	err := wssc.conn.WriteJSON(&req)
	wssc.mu.Unlock()
	if err != nil {
		LogWarn("[wss] failed to send api request to websocket server: %v", err.Error())
		return nullResponse, err
	}
	LogDebug("[wss] sending api request to server: %v", &req)

	select {
	case rsp, ok := <-ch:
		if !ok {
			return nullResponse, io.ErrClosedPipe
		}
		return rsp, nil
	case <-time.After(time.Minute):
		return nullResponse, os.ErrDeadlineExceeded
	}
}
