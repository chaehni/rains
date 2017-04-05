package rainsd

import (
	"crypto/x509"
	"net"
	"rains/rainslib"
	"strconv"
	"sync"
	"time"
)

var serverConnInfo ConnInfo
var roots *x509.CertPool
var msgParser rainslib.RainsMsgParser

//Config contains configurations for this server
var Config = defaultConfig

//rainsdConfig lists possible configurations of a rains server
type rainsdConfig struct {
	//switchboard
	ServerIPAddr          net.IP
	ServerPort            uint16
	MaxConnections        uint
	KeepAlivePeriodMicros time.Duration
	TCPTimeoutMicros      time.Duration
	CertificateFile       string
	PrivateKeyFile        string

	//inbox
	MaxMsgByteLength        uint
	PrioBufferSize          uint
	NormalBufferSize        uint
	NotificationBufferSize  uint
	PrioWorkerCount         uint
	NormalWorkerCount       uint
	NotificationWorkerCount uint
	CapabilitiesCacheSize   uint
	PeerToCapCacheSize      uint

	//verify
	ZoneKeyCacheSize          uint
	PendingSignatureCacheSize uint

	//engine
	AssertionCacheSize    uint
	PendingQueryCacheSize uint
}

//DefaultConfig is a rainsdConfig object containing default values
var defaultConfig = rainsdConfig{ServerIPAddr: net.ParseIP("127.0.0.1"), ServerPort: 5022, MaxConnections: 1000, KeepAlivePeriodMicros: time.Minute, TCPTimeoutMicros: 5 * time.Minute,
	CertificateFile: "config/server.crt", PrivateKeyFile: "config/server.key", MaxMsgByteLength: 65536, PrioBufferSize: 1000, NormalBufferSize: 100000, PrioWorkerCount: 2,
	NormalWorkerCount: 10, ZoneKeyCacheSize: 1000, PendingSignatureCacheSize: 1000, AssertionCacheSize: 10000, PendingQueryCacheSize: 100, CapabilitiesCacheSize: 50,
	NotificationBufferSize: 20, NotificationWorkerCount: 2, PeerToCapCacheSize: 1000}

//ProtocolType enumerates protocol types
type ProtocolType int

const (
	TCP ProtocolType = iota
)

//ConnInfo contains address information about one actor of a connection of the declared type
type ConnInfo struct {
	Type   ProtocolType
	IPAddr net.IP
	Port   uint16
}

//IPAddrAndPort returns IP address and port in the format IPAddr:Port
func (c ConnInfo) IPAddrAndPort() string {
	return c.IPAddr.String() + ":" + c.PortToString()
}

//PortToString return the port number as a string
func (c ConnInfo) PortToString() string {
	return strconv.Itoa(int(c.Port))
}

//msgSectionSender contains the message section section and connection infos about the sender
type msgSectionSender struct {
	Sender ConnInfo
	Msg    rainslib.MessageSection
	Token  rainslib.Token
}

//Capability is a type which defines what a server or client is capable of
type Capability string

const (
	NoCapability Capability = ""
	TLSOverTCP   Capability = "urn:x-rains:tlssrv"
)

type keyCacheKey struct {
	context string
	zone    string
	keyAlgo rainslib.KeyAlgorithmType
}

//keyCache is the Interface which must be implemented by all caches for keys.
type keyCache interface {
	//Add adds a keyCacheValue to the cache. If the cache is full the oldest element according to some metric will be replaced. Returns true if it was able to add the public key.
	Add(key keyCacheKey, value rainslib.PublicKey) bool
	//Get returns a valid public key matching the given cacheKey. It returns false if there exists no valid public key in the cache.
	//Get must always check the validity of the public key before returning.
	Get(key keyCacheKey) (rainslib.PublicKey, bool)
	//Keys returns all keyCacheKeys present in the cache
	Keys() []keyCacheKey
	//Len returns the number of elements in the cache.
	Len() int
	//Remove deletes the given key from the cache together with its associated value
	Remove(key keyCacheKey)
	//RemoveWithStrategy deletes a key value pair from the cache according to some strategy
	RemoveWithStrategy()
}

//connectionCache stores all active connections
type connectionCache interface {
	//Add adds a new connection to the cash. If for the given fourTuple there is already a connection in the cache, the connection gets replaced with the new one.
	//Returns false if the cache already contained an entry for the fourTuple.
	//If the cache is full it closes and removes a connection according to some metric
	Add(fourTuple string, conn net.Conn) bool
	//Get returns a connection associated with the given four tuple.
	//If there is an element in the cache its recentness will be updated
	//Returns false if there is no connection for the given fourTuple in the cache.
	Get(fourTuple string) (net.Conn, bool)
	//Len returns the number of elements in the cache.
	Len() int
}

//capabilityCache contains known capabilities
type capabilityCache interface {
	//Add adds the capabilities to the cash and creates or updates a mapping between the capabilities and the hash thereof.
	//Returns true if the given connInfo was not yet in the cache and false if it updated the capabilities and the recentness of the entry for connInfo.
	//If the cache is full it removes a capability according to some metric
	Add(connInfo ConnInfo, capabilities []rainslib.Capability) bool
	//Get returns all capabilities associated with the given connInfo and updates the recentness of the entry.
	//It returns false if there exists no entry for connInfo
	Get(connInfo ConnInfo) ([]rainslib.Capability, bool)
	//GetFromHash returns true and the capabilities from which the hash was taken if present, otherwise false
	GetFromHash(hash []byte) ([]rainslib.Capability, bool)
}

//pendingSignatureCacheValue is the value received from the pendingQuery cache
type pendingSignatureCacheValue struct {
	section    rainslib.MessageSectionWithSig
	ValidUntil int64
}

//pendingSignatureCache stores all sections with a signature waiting for a public key to arrive so they can be verified
type pendingSignatureCache interface {
	//Add adds a section together with a validity to the cache. Returns true if there is not yet a pending query for this request
	//If the cache is full it removes a section according to some metric.
	Add(context, zone string, section pendingSignatureCacheValue) bool
	//Get returns all still valid sections associated with the given context and zone. It returns an empty list if there exists no valid section
	Get(context, zone string) []rainslib.MessageSectionWithSig
	//RemoveExpiredSections goes through the cache and removes all invalid sections. If for a given context and zone there is no section left it removes the entry from cache.
	RemoveExpiredSections()
	//Len returns the number of elements in the cache.
	Len() int
}

//pendingSignatureCacheValue is the value received from the pendingQuery cache
type pendingQueryCacheValue struct {
	ConnInfo   ConnInfo
	Token      rainslib.Token //Token from the received query
	ValidUntil int64
}

//pendingQueryCache stores connection information about queriers which are waiting for an assertion to arrive
type pendingQueryCache interface {
	//Add adds connection information together with a token and a validity to the cache.
	//Returns true and a token for the query if cache did not already contain an entry for context,zone,name,objType else return false
	//If the cache is full it removes a pendingQueryCacheValue according to some metric.
	Add(context, zone, name string, objType rainslib.ObjectType, section pendingQueryCacheValue) (bool, rainslib.Token)
	//Get returns all valid pendingQueryCacheValues associated with the given token. It returns false if there exists no valid value.
	Get(token rainslib.Token) []pendingQueryCacheValue
	//RemoveExpiredValues goes through the cache and removes all expired values and tokens. If for a given context and zone there is no value left it removes the entry from cache.
	RemoveExpiredValues()
	//Len returns the number of elements in the cache.
	Len() int
}

//negativeAssertionCacheValue is the value stored in the negativeAssertionCache
type negativeAssertionCacheValue struct {
	section    rainslib.MessageSectionWithSig
	validFrom  int64
	ValidUntil int64
}

type negativeAssertionCache interface {
	//Add adds a shard or zone together with a validity to the cache.
	//Returns true if cache did not already contain an entry for the given context and zone
	//If the cache is full it removes an external negativeAssertionCacheValue according to some metric.
	Add(context, zone string, internal bool, value negativeAssertionCacheValue) bool
	//Get returns all sections of a given context and zone which intersect with the given Range. Returns an empty list if there are none
	//if beginRange and endRange are an empty string then the zone and all shards of that context and zone are returned
	Get(context, zone, beginRange, endRange string) []rainslib.MessageSectionWithSig
	//Len returns the number of elements in the cache.
	Len() int
	//RemoveExpiredValues goes through the cache and removes all expired values. If for a given context and zone there is no value left it removes the entry from cache.
	RemoveExpiredValues()
}

//assertionCacheValue is the value stored in the assertionCacheValue
type assertionCacheValue struct {
	section    *rainslib.AssertionSection
	validFrom  int64
	ValidUntil int64
}

//assertionCache is used to store and efficiently lookup assertions
type assertionCache interface {
	//Add adds an assertion together with a validity to the cache.
	//Returns true if cache did not already contain an entry for the given context,zone, name and objType
	//If the cache is full it removes an external assertionCacheValue according to some metric.
	Add(context, zone, name string, objType rainslib.ObjectType, internal bool, value assertionCacheValue) bool
	//Get returns a set of valid assertions matching the given key. Returns an empty list if there are none
	Get(context, zone, name string, objType rainslib.ObjectType) []*rainslib.AssertionSection
	//GetInRange returns a set of valid assertions in the range [beginRange, endRange] matching the given context and zone. Returns an empty list if there are none.
	GetInRange(context, zone, beginRange, endRange string) []*rainslib.AssertionSection
	//Len returns the number of elements in the cache.
	Len() int
	//RemoveExpiredValues goes through the cache and removes all expired values. If for a given context and zone there is no value left it removes the entry from cache.
	//If for a given context, zone, name and object type there is no value left it removes the entry from cache.
	RemoveExpiredValues()
}

//contextAndZone stores a context and a zone
type contextAndZone struct {
	Context string
	Zone    string
}

/*
//pendingSignatureCacheValue is the value received from the pendingQuery cache
type pendingSignatureCacheValue struct {
	ValidUntil     int64
	retries        int
	mux            sync.Mutex
	MsgSectionList msgSectionWithSigList
}

//Retries returns the number of retries. If 0 no retries are attempted
func (v *pendingSignatureCacheValue) Retries() int {
	v.mux.Lock()
	defer func(v *pendingSignatureCacheValue) { v.mux.Unlock() }(v)
	return v.retries
}

//DecRetries decreses the retry value by 1
func (v *pendingSignatureCacheValue) DecRetries() {
	v.mux.Lock()
	if v.retries > 0 {
		v.retries--
	}
}*/

type queryAnswerList struct {
	ConnInfo ConnInfo
	Token    rainslib.Token
}

//msgSectionWithSigList is a thread safe list of msgSectionWithSig
//To handle the case that we do not drop an incoming msgSection during the handling of the callback, we close the list after callback and return false
//Then the calling method can handle the new msgSection directly.
type msgSectionWithSigList struct {
	mux                   sync.Mutex
	closed                bool
	MsgSectionWithSigList []rainslib.MessageSectionWithSig
}

//Add adds an message section with signature to the list (It is thread safe)
//returns true if it was able to add the element to the list
func (l *msgSectionWithSigList) Add(section rainslib.MessageSectionWithSig) bool {
	l.mux.Lock()
	defer func(l *msgSectionWithSigList) { l.mux.Unlock() }(l)
	if !l.closed {
		l.MsgSectionWithSigList = append(l.MsgSectionWithSigList, section)
		return true
	}
	return false
}

//GetList returns the list and closes the data structure
func (l *msgSectionWithSigList) GetListAndClose() []rainslib.MessageSectionWithSig {
	l.mux.Lock()
	defer func(l *msgSectionWithSigList) { l.mux.Unlock() }(l)
	l.closed = true
	return l.MsgSectionWithSigList
}

//TODO CFE what should the name of this interface be?
type scanner interface {
	//Frame takes a message and adds a frame to it
	Frame(msg []byte) ([]byte, error)

	//Deframe extracts the next frame from a stream.
	//It blocks until it encounters the delimiter.
	//It returns false when the stream is closed.
	//The data is available through Data
	Deframe() bool

	//Data contains the frame read from the stream by Deframe
	Data() []byte
}

//container is an interface for a map data structure which might be concurrency safe
type container interface {
	//Add appends item to the current list.
	//It returns false if it was not able to add the element because the underlying datastructure was deleted in the meantime
	Add(item interface{}) bool

	//Delete removes item from the list.
	Delete(item interface{})

	//GetAll returns all elements contained in the datastructure
	GetAll() []interface{}

	//GetAllAndDelete returns all contained elements and deletes the datastructure.
	GetAllAndDelete() []interface{}
}
