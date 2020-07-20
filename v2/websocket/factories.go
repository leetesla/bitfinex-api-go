package websocket

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/bitfinexcom/bitfinex-api-go/pkg/convert"
	"github.com/bitfinexcom/bitfinex-api-go/pkg/models/candle"
	"github.com/bitfinexcom/bitfinex-api-go/pkg/models/derivatives"
	"github.com/bitfinexcom/bitfinex-api-go/pkg/models/ticker"
	"github.com/bitfinexcom/bitfinex-api-go/v2"
)

type messageFactory interface {
	Build(sub *subscription, objType string, raw []interface{}, raw_bytes []byte) (interface{}, error)
	BuildSnapshot(sub *subscription, raw [][]interface{}, raw_bytes []byte) (interface{}, error)
}

type TickerFactory struct {
	*subscriptions
}

func newTickerFactory(subs *subscriptions) *TickerFactory {
	return &TickerFactory{
		subscriptions: subs,
	}
}

func (f *TickerFactory) Build(sub *subscription, objType string, raw []interface{}, raw_bytes []byte) (interface{}, error) {
	return ticker.FromRaw(sub.Request.Symbol, raw)
}

func (f *TickerFactory) BuildSnapshot(sub *subscription, raw [][]interface{}, raw_bytes []byte) (interface{}, error) {
	return ticker.SnapshotFromRaw(sub.Request.Symbol, raw)
}

type TradeFactory struct {
	*subscriptions
}

func newTradeFactory(subs *subscriptions) *TradeFactory {
	return &TradeFactory{
		subscriptions: subs,
	}
}

func (f *TradeFactory) Build(sub *subscription, objType string, raw []interface{}, raw_bytes []byte) (interface{}, error) {
	if "tu" == objType {
		return nil, nil // do not process TradeUpdate messages on public feed, only need to process TradeExecution (first copy seen)
	}
	return bitfinex.NewTradeFromRaw(sub.Request.Symbol, raw)
}

func (f *TradeFactory) BuildSnapshot(sub *subscription, raw [][]interface{}, raw_bytes []byte) (interface{}, error) {
	converted, err := convert.ToFloat64Array(raw)
	if err != nil {
		return nil, err
	}
	return bitfinex.NewTradeSnapshotFromRaw(sub.Request.Symbol, converted)
}

type BookFactory struct {
	*subscriptions
	orderbooks  map[string]*Orderbook
	manageBooks bool
	lock        sync.Mutex
}

func newBookFactory(subs *subscriptions, obs map[string]*Orderbook, manageBooks bool) *BookFactory {
	return &BookFactory{
		subscriptions: subs,
		orderbooks:    obs,
		manageBooks:   manageBooks,
	}
}

func ConvertBytesToJsonNumberArray(raw_bytes []byte) ([]interface{}, error) {
	var raw_json_number []interface{}
	d := json.NewDecoder(strings.NewReader(string(raw_bytes)))
	d.UseNumber()
	str_conv_err := d.Decode(&raw_json_number)
	if str_conv_err != nil {
		return nil, str_conv_err
	}
	return raw_json_number, nil
}

func (f *BookFactory) Build(sub *subscription, objType string, raw []interface{}, raw_bytes []byte) (interface{}, error) {
	// we need ot parse the bytes using json numbers since they store the exact string value
	// and not a float64 representation
	raw_json_number, str_conv_err := ConvertBytesToJsonNumberArray(raw_bytes)
	if str_conv_err != nil {
		return nil, str_conv_err
	}

	update, err := bitfinex.NewBookUpdateFromRaw(sub.Request.Symbol, sub.Request.Precision, raw, raw_json_number[1])
	if f.manageBooks {
		f.lock.Lock()
		defer f.lock.Unlock()
		if orderbook, ok := f.orderbooks[sub.Request.Symbol]; ok {
			orderbook.UpdateWith(update)
		}
	}
	return update, err
}

func (f *BookFactory) BuildSnapshot(sub *subscription, raw [][]interface{}, raw_bytes []byte) (interface{}, error) {
	converted, err := convert.ToFloat64Array(raw)
	if err != nil {
		return nil, err
	}
	// parse the bytes using the json number value to store the exact string value
	raw_json_number, str_conv_err := ConvertBytesToJsonNumberArray(raw_bytes)
	if str_conv_err != nil {
		return nil, str_conv_err
	}

	update, err2 := bitfinex.NewBookUpdateSnapshotFromRaw(sub.Request.Symbol, sub.Request.Precision, converted, raw_json_number[1])
	if err2 != nil {
		return nil, err2
	}
	if f.manageBooks {
		f.lock.Lock()
		defer f.lock.Unlock()
		// create new orderbook
		f.orderbooks[sub.Request.Symbol] = &Orderbook{
			symbol: sub.Request.Symbol,
			bids:   make([]*bitfinex.BookUpdate, 0),
			asks:   make([]*bitfinex.BookUpdate, 0),
		}
		f.orderbooks[sub.Request.Symbol].SetWithSnapshot(update)
	}
	return update, err
}

type CandlesFactory struct {
	*subscriptions
}

func newCandlesFactory(subs *subscriptions) *CandlesFactory {
	return &CandlesFactory{
		subscriptions: subs,
	}
}

func (f *CandlesFactory) Build(sub *subscription, objType string, raw []interface{}, raw_bytes []byte) (interface{}, error) {
	sym, res, err := extractSymbolResolutionFromKey(sub.Request.Key)
	if err != nil {
		return nil, err
	}
	candle, err := candle.FromRaw(sym, res, raw)
	return candle, err
}

func (f *CandlesFactory) BuildSnapshot(sub *subscription, raw [][]interface{}, raw_bytes []byte) (interface{}, error) {
	sym, res, err := extractSymbolResolutionFromKey(sub.Request.Key)
	if err != nil {
		return nil, err
	}
	snap, err := candle.SnapshotFromRaw(sym, res, raw)
	return snap, err
}

type StatsFactory struct {
	*subscriptions
}

func newStatsFactory(subs *subscriptions) *StatsFactory {
	return &StatsFactory{
		subscriptions: subs,
	}
}

func (f *StatsFactory) Build(sub *subscription, objType string, raw []interface{}, raw_bytes []byte) (interface{}, error) {
	splits := strings.Split(sub.Request.Key, ":")
	if len(splits) != 3 {
		return nil, fmt.Errorf("unable to parse key to symbol %s", sub.Request.Key)
	}
	symbol := splits[1] + ":" + splits[2]
	d, err := derivatives.FromWsRaw(symbol, raw)
	return d, err
}

func (f *StatsFactory) BuildSnapshot(sub *subscription, raw [][]interface{}, raw_bytes []byte) (interface{}, error) {
	// no snapshots
	return nil, nil
}
