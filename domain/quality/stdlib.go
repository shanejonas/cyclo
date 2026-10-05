package quality

import "slices"

// stdlibRule classifies one Go standard library package for side-effect
// analysis. When names is empty the kind applies to every call in the
// package (prefix "pkg."). When names is set only "pkg."+name entries get
// the kind, for packages mixing pure and effectful calls. Longer prefixes
// win, so narrow entries override blankets (e.g. "encoding/json.Encoder.Encode"
// stays IO under an "encoding/" None blanket).
//
// The goal is that no standard library call falls through to Unknown:
// pure calls are None, effectful calls get their real kind. Unknown then
// means "a call cyclo cannot see into" — third-party code or dynamic
// dispatch — which is a legitimate signal, not noise.
type stdlibRule struct {
	pkg   string
	kind  Kind
	names []string
}

// stdlibRules is the curated classification of the Go standard library.
// TestStdlibCoverage fails if go list std reports a package with no entry
// here, so newly added stdlib packages force a classification decision.
var stdlibRules = []stdlibRule{
	// Pure data transformation: strings, bytes, collections, math.
	{pkg: "strings", kind: None},
	{pkg: "bytes", kind: None},
	{pkg: "strconv", kind: None},
	{pkg: "unicode", kind: None},
	{pkg: "unicode/utf8", kind: None},
	{pkg: "unicode/utf16", kind: None},
	{pkg: "cmp", kind: None},
	{pkg: "slices", kind: None},
	{pkg: "maps", kind: None},
	{pkg: "sort", kind: None},
	{pkg: "math", kind: None},
	{pkg: "math/bits", kind: None},
	{pkg: "math/cmplx", kind: None},
	{pkg: "math/big", kind: None},
	{pkg: "errors", kind: None},
	{pkg: "path", kind: None},
	{pkg: "iter", kind: None},
	{pkg: "unique", kind: None},
	{pkg: "weak", kind: None},
	{pkg: "structs", kind: None},
	{pkg: "container/heap", kind: None},
	{pkg: "container/list", kind: None},
	{pkg: "container/ring", kind: None},
	{pkg: "index/suffixarray", kind: None},

	// Pure codecs and parsing. Encoder/Writer types that perform IO get
	// narrow overrides below.
	{pkg: "encoding", kind: None},
	{pkg: "encoding/ascii85", kind: None},
	{pkg: "encoding/asn1", kind: None},
	{pkg: "encoding/base32", kind: None},
	{pkg: "encoding/base64", kind: None},
	{pkg: "encoding/binary", kind: None},
	{pkg: "encoding/hex", kind: None},
	{pkg: "encoding/pem", kind: None},
	{pkg: "encoding/csv", kind: None},
	{pkg: "encoding/csv", kind: IO, names: []string{
		"Reader.Read", "Reader.ReadAll",
		"Writer.Write", "Writer.WriteAll", "Writer.Flush",
	}},
	{pkg: "encoding/gob", kind: None},
	{pkg: "encoding/gob", kind: IO, names: []string{"Encoder.Encode", "Decoder.Decode"}},
	{pkg: "encoding/gob", kind: Global, names: []string{"Register", "RegisterName"}},
	{pkg: "encoding/json", kind: None},
	{pkg: "encoding/json", kind: IO, names: []string{
		"Encoder.Encode", "Decoder.Decode", "Decoder.Token", "Decoder.More",
	}},
	{pkg: "encoding/json/v2", kind: None},
	{pkg: "encoding/json/jsontext", kind: None},
	{pkg: "encoding/xml", kind: None},
	{pkg: "encoding/xml", kind: IO, names: []string{
		"Encoder.Encode", "Decoder.Decode", "Decoder.Token", "Decoder.RawToken",
	}},

	// Hashing and crypto primitives are pure data transforms. rand stays
	// Random; tls.Dial opens real connections.
	{pkg: "crypto", kind: None},
	{pkg: "crypto/aes", kind: None},
	{pkg: "crypto/cipher", kind: None},
	{pkg: "crypto/des", kind: None},
	{pkg: "crypto/dsa", kind: None},
	{pkg: "crypto/ecdh", kind: None},
	{pkg: "crypto/ecdsa", kind: None},
	{pkg: "crypto/ed25519", kind: None},
	{pkg: "crypto/elliptic", kind: None},
	{pkg: "crypto/fips140", kind: None},
	{pkg: "crypto/hkdf", kind: None},
	{pkg: "crypto/hmac", kind: None},
	{pkg: "crypto/hpke", kind: None},
	{pkg: "crypto/md5", kind: None},
	{pkg: "crypto/mldsa", kind: None},
	{pkg: "crypto/mlkem", kind: None},
	{pkg: "crypto/mlkem/mlkemtest", kind: None},
	{pkg: "crypto/pbkdf2", kind: None},
	{pkg: "crypto/rc4", kind: None},
	{pkg: "crypto/rsa", kind: None},
	{pkg: "crypto/sha1", kind: None},
	{pkg: "crypto/sha256", kind: None},
	{pkg: "crypto/sha3", kind: None},
	{pkg: "crypto/sha512", kind: None},
	{pkg: "crypto/subtle", kind: None},
	{pkg: "crypto/rand", kind: Random},
	{pkg: "math/rand", kind: Random},
	{pkg: "math/rand/v2", kind: Random},
	{pkg: "crypto/tls", kind: None},
	{pkg: "crypto/tls", kind: Network, names: []string{"Dial", "DialWithDialer"}},
	{pkg: "crypto/x509", kind: None},
	{pkg: "crypto/x509/pkix", kind: None},
	{pkg: "hash", kind: None},
	{pkg: "hash/adler32", kind: None},
	{pkg: "hash/crc32", kind: None},
	{pkg: "hash/crc64", kind: None},
	{pkg: "hash/fnv", kind: None},
	{pkg: "hash/maphash", kind: None},

	// Compression: constructors and pure helpers are None; the
	// Reader/Writer methods move bytes and are IO.
	{pkg: "compress/bzip2", kind: None},
	{pkg: "compress/bzip2", kind: IO, names: []string{"Reader.Read", "Reader.Close"}},
	{pkg: "compress/flate", kind: None},
	{pkg: "compress/flate", kind: IO, names: []string{"Reader.Read", "Reader.Close", "Writer.Write", "Writer.Flush", "Writer.Close"}},
	{pkg: "compress/gzip", kind: None},
	{pkg: "compress/gzip", kind: IO, names: []string{
		"NewReader", "Reader.Read", "Reader.Close",
		"Writer.Write", "Writer.Flush", "Writer.Close",
	}},
	{pkg: "compress/lzw", kind: None},
	{pkg: "compress/lzw", kind: IO, names: []string{"Reader.Read", "Reader.Close", "Writer.Write", "Writer.Close"}},
	{pkg: "compress/zlib", kind: None},
	{pkg: "compress/zlib", kind: IO, names: []string{"Reader.Read", "Reader.Close", "Writer.Write", "Writer.Flush", "Writer.Close"}},
	{pkg: "archive/tar", kind: None},
	{pkg: "archive/tar", kind: IO, names: []string{
		"Reader.Next", "Reader.Read", "Reader.Close",
		"Writer.Write", "Writer.WriteHeader", "Writer.Flush", "Writer.Close",
	}},
	{pkg: "archive/zip", kind: None},
	{pkg: "archive/zip", kind: IO, names: []string{
		"OpenReader", "NewReader", "Writer.Write", "Writer.Close",
		"File.Open", "ReadCloser.Close",
	}},

	// Buffered IO wrappers: constructors pure, methods that move bytes IO.
	{pkg: "bufio", kind: None},
	{pkg: "bufio", kind: IO, names: []string{
		"Reader.Read", "Reader.ReadByte", "Reader.ReadBytes", "Reader.ReadRune",
		"Reader.ReadSlice", "Reader.ReadString", "Reader.Discard", "Reader.Peek",
		"Writer.Write", "Writer.WriteByte", "Writer.WriteRune", "Writer.WriteString",
		"Writer.Flush", "Writer.ReadFrom",
		"Scanner.Scan",
	}},

	// Core IO.
	{pkg: "io", kind: IO, names: []string{
		"Copy", "CopyN", "CopyBuffer", "ReadAll", "ReadFull", "ReadAtLeast",
		"WriteString", "Pipe",
	}},
	{pkg: "io", kind: None, names: []string{
		"LimitReader", "MultiReader", "TeeReader", "SectionReader", "NewSectionReader",
		"MultiWriter",
	}},
	{pkg: "io", kind: IO, names: []string{"Reader.Read", "Writer.Write", "Closer.Close", "Seeker.Seek"}},
	{pkg: "io/fs", kind: IO, names: []string{
		"ReadFile", "WriteFile", "ReadDir", "Stat", "Glob", "WalkDir", "Mkdir",
	}},
	{pkg: "io/fs", kind: None, names: []string{"ValidPath"}},
	{pkg: "io/ioutil", kind: IO, names: []string{
		"ReadFile", "WriteFile", "ReadDir", "ReadAll", "TempDir", "TempFile",
	}},
	{pkg: "io/ioutil", kind: None, names: []string{"NopCloser", "Discard"}},

	// OS: narrow, like before. Getenv-style process reads are IO;
	// pure identity queries are None.
	{pkg: "os", kind: IO, names: []string{
		"Open", "OpenFile", "Create", "ReadFile", "WriteFile", "ReadDir",
		"Mkdir", "MkdirAll", "MkdirTemp", "CreateTemp", "Remove", "RemoveAll",
		"Rename", "Stat", "Lstat", "Chmod", "Chown", "Chtimes", "Truncate",
		"Link", "Symlink", "Readlink", "Getenv", "LookupEnv", "Environ",
		"Setenv", "Unsetenv", "Clearenv", "Getwd", "Chdir", "Exit",
		"StartProcess", "FindProcess", "Pipe", "NewFile",
	}},
	{pkg: "os", kind: None, names: []string{
		"Getpid", "Getppid", "Getuid", "Geteuid", "Getgid", "Getegid",
		"Getpagesize", "Args",
	}},
	{pkg: "os", kind: IO, names: []string{
		"File.Read", "File.ReadAt", "File.Write", "File.WriteAt", "File.WriteString",
		"File.Close", "File.Seek", "File.Sync", "File.Stat", "File.Truncate",
		"File.Chmod", "File.Chown", "File.Readdir", "File.ReadDir", "File.Name",
	}},
	{pkg: "os/exec", kind: None, names: []string{"Command", "CommandContext"}},
	{pkg: "os/exec", kind: IO, names: []string{
		"Cmd.Run", "Cmd.Start", "Cmd.Wait", "Cmd.Output", "Cmd.CombinedOutput",
		"Cmd.StdinPipe", "Cmd.StdoutPipe", "Cmd.StderrPipe",
	}},
	{pkg: "os/signal", kind: Global, names: []string{
		"Notify", "NotifyContext", "Stop", "Reset", "Ignore",
	}},
	{pkg: "os/user", kind: IO, names: []string{
		"Current", "Lookup", "LookupId", "LookupGroup", "LookupGroupId",
	}},
	{pkg: "syscall", kind: IO},
	{pkg: "syscall", kind: Unsafe, names: []string{"Syscall", "Syscall6", "RawSyscall", "RawSyscall6"}},
	{pkg: "plugin", kind: IO, names: []string{"Open"}},

	// Paths: pure manipulation. Abs/Glob/EvalSymlinks consult the machine.
	{pkg: "path", kind: None},
	{pkg: "path/filepath", kind: None, names: []string{
		"Base", "Dir", "Ext", "Join", "Split", "Clean", "IsAbs", "Rel",
		"Match", "SplitList", "ToSlash", "FromSlash", "VolumeName",
	}},

	// Formatting: builders pure, writers IO.
	{pkg: "fmt", kind: IO, names: []string{
		"Print", "Printf", "Println",
		"Fprint", "Fprintf", "Fprintln",
		"Scan", "Scanf", "Scanln", "Sscan", "Sscanf", "Sscanln",
		"Fscan", "Fscanf", "Fscanln",
	}},
	{pkg: "fmt", kind: None, names: []string{
		"Sprint", "Sprintf", "Sprintln", "Errorf", "Append", "Appendf", "Appendln",
	}},

	// Logging goes to IO; constructors and level/attr builders are pure.
	{pkg: "log", kind: IO, names: []string{
		"Print", "Printf", "Println", "Fatal", "Fatalf", "Fatalln",
		"Panic", "Panicf", "Panicln", "Output",
		"Logger.Print", "Logger.Printf", "Logger.Println",
		"Logger.Fatal", "Logger.Fatalf", "Logger.Fatalln",
		"Logger.Panic", "Logger.Panicf", "Logger.Panicln", "Logger.Output",
	}},
	{pkg: "log", kind: Global, names: []string{
		"SetOutput", "SetPrefix", "SetFlags",
		"Logger.SetOutput", "Logger.SetPrefix", "Logger.SetFlags",
	}},
	{pkg: "log", kind: None, names: []string{"New", "Default", "Prefix", "Flags", "Writer"}},
	{pkg: "log/slog", kind: IO, names: []string{
		"Debug", "Info", "Warn", "Error", "Log",
		"Logger.Debug", "Logger.Info", "Logger.Warn", "Logger.Error", "Logger.Log",
		"Logger.Handler",
	}},
	{pkg: "log/slog", kind: Global, names: []string{"SetDefault"}},
	{pkg: "log/slog", kind: None, names: []string{
		"New", "Default", "NewRecord", "NewTextHandler", "NewJSONHandler",
		"Any", "Bool", "Duration", "Float64", "Group", "Int", "Int64",
		"String", "Time", "Uint64", "With",
		"Logger.With", "Record.Add", "Record.AddAttrs",
	}},
	{pkg: "log/syslog", kind: Network, names: []string{"Dial"}},
	{pkg: "log/syslog", kind: IO, names: []string{
		"New", "Writer.Write", "Writer.Close", "Writer.Alert", "Writer.Crit",
		"Writer.Debug", "Writer.Emerg", "Writer.Err", "Writer.Info",
		"Writer.Notice", "Writer.Warning",
	}},

	// Time: clocks are Time, location loading reads the machine, the rest
	// (arithmetic, formatting, parsing) is pure.
	{pkg: "time", kind: Time, names: []string{
		"Now", "Since", "Until", "Sleep", "Tick",
		"NewTimer", "NewTicker", "After", "AfterFunc",
	}},
	{pkg: "time", kind: IO, names: []string{"LoadLocation"}},
	{pkg: "time", kind: None, names: []string{
		"Parse", "ParseInLocation", "Date", "Unix", "UnixMilli", "UnixMicro",
		"Time.", "Duration.", "Month.", "Weekday.",
	}},
	{pkg: "time/tzdata", kind: None},

	// Network: dialing and serving are Network; parsing and address
	// arithmetic are pure.
	{pkg: "net", kind: Network, names: []string{
		"Dial", "DialTimeout", "Listen", "ListenPacket",
		"ResolveIPAddr", "ResolveTCPAddr", "ResolveUDPAddr", "ResolveUnixAddr",
		"LookupHost", "LookupIP", "LookupPort", "LookupCNAME", "LookupMX",
		"LookupNS", "LookupTXT", "LookupSRV", "LookupAddr",
		"InterfaceAddrs", "Interfaces", "Pipe",
		"Conn.", "TCPConn.", "UDPConn.", "UnixConn.", "IPConn.",
		"Listener.Accept", "Listener.Close", "Listener.Addr",
		"PacketConn.",
	}},
	{pkg: "net", kind: None, names: []string{
		"ParseIP", "ParseCIDR", "ParseMAC",
		"SplitHostPort", "JoinHostPort",
		"IPv4", "IPv4Mask", "CIDRMask",
		"IP.", "IPNet.", "TCPAddr.", "UDPAddr.", "UnixAddr.",
	}},
	{pkg: "net/netip", kind: None},
	{pkg: "net/url", kind: None},
	{pkg: "net/mail", kind: None},
	{pkg: "net/textproto", kind: None},
	{pkg: "net/textproto", kind: Network, names: []string{"Dial"}},
	{pkg: "net/smtp", kind: Network, names: []string{"SendMail", "Dial"}},
	{pkg: "net/smtp", kind: None, names: []string{"PlainAuth", "CRAMMD5Auth"}},
	{pkg: "net/rpc", kind: Network, names: []string{"Dial", "DialHTTP", "DialHTTPPath", "Accept"}},
	{pkg: "net/rpc", kind: Global, names: []string{"Register", "HandleHTTP"}},
	{pkg: "net/rpc", kind: None, names: []string{"NewServer", "NewClient", "Server.Register"}},
	{pkg: "net/rpc/jsonrpc", kind: Network, names: []string{"Dial"}},
	{pkg: "net/rpc/jsonrpc", kind: None, names: []string{"NewClientCodec", "NewServerCodec"}},
	{pkg: "net/http", kind: Network, names: []string{
		"Get", "Head", "Post", "PostForm",
		"ListenAndServe", "ListenAndServeTLS", "Serve",
		"Client.Do", "Client.Get", "Client.Head", "Client.Post", "Client.PostForm",
		"Server.Serve", "Server.ServeTLS", "Server.ListenAndServe", "Server.ListenAndServeTLS",
		"Transport.RoundTrip",
	}},
	{pkg: "net/http", kind: Global, names: []string{"Handle", "HandleFunc"}},
	{pkg: "net/http", kind: IO, names: []string{
		"ServeFile", "ServeContent", "ServeCookie", "Error", "Redirect",
		"Request.ParseForm", "Request.ParseMultipartForm",
	}},
	{pkg: "net/http", kind: None, names: []string{
		"NewRequest", "NewRequestWithContext", "NewServeMux", "NewResponseController",
		"Request.WithContext", "Request.Context", "Request.FormValue", "Request.URL",
		"CanonicalHeaderKey", "DetectContentType", "ParseTime", "StatusText",
	}},
	{pkg: "net/http/cgi", kind: Network, names: []string{"Serve"}},
	{pkg: "net/http/cgi", kind: None, names: []string{"Handler.ServeHTTP"}},
	{pkg: "net/http/fcgi", kind: Network, names: []string{"Serve"}},
	{pkg: "net/http/cookiejar", kind: None},
	{pkg: "net/http/httptest", kind: Network, names: []string{"NewServer", "NewTLSServer"}},
	{pkg: "net/http/httptest", kind: None, names: []string{"NewRecorder", "NewRequest"}},
	{pkg: "net/http/httptrace", kind: None},
	{pkg: "net/http/httputil", kind: None},
	{pkg: "net/http/httputil", kind: IO, names: []string{"DumpRequest", "DumpRequestOut", "DumpResponse"}},
	{pkg: "net/http/pprof", kind: None},

	// Database: opening is lazy and pure; queries hit the store.
	{pkg: "database/sql", kind: None},
	{pkg: "database/sql", kind: IO, names: []string{
		"DB.Query", "DB.QueryContext", "DB.QueryRow", "DB.QueryRowContext",
		"DB.Exec", "DB.ExecContext", "DB.Prepare", "DB.PrepareContext",
		"DB.Begin", "DB.BeginTx", "DB.Ping", "DB.PingContext",
		"Tx.Commit", "Tx.Rollback", "Tx.Query", "Tx.Exec", "Tx.Prepare", "Tx.Stmt",
		"Stmt.Query", "Stmt.QueryRow", "Stmt.Exec",
		"Rows.Next", "Rows.Scan", "Rows.Close",
	}},
	{pkg: "database/sql/driver", kind: None},

	// Reflection, regexps, templates: inspection and building are pure;
	// executing a template against a writer is IO.
	{pkg: "reflect", kind: None},
	{pkg: "regexp", kind: None},
	{pkg: "regexp/syntax", kind: None},
	{pkg: "html", kind: None},
	{pkg: "html/template", kind: None},
	{pkg: "html/template", kind: IO, names: []string{"Template.Execute", "Template.ExecuteTemplate"}},
	{pkg: "text/template", kind: None},
	{pkg: "text/template", kind: IO, names: []string{"Template.Execute", "Template.ExecuteTemplate"}},
	{pkg: "text/template/parse", kind: None},
	{pkg: "text/scanner", kind: None},
	{pkg: "text/tabwriter", kind: None},
	{pkg: "text/tabwriter", kind: IO, names: []string{"Writer.Write", "Writer.Flush"}},

	// Images: decoding reads, encoding writes, the rest is pure.
	{pkg: "image", kind: None},
	{pkg: "image/color", kind: None},
	{pkg: "image/color/palette", kind: None},
	{pkg: "image/draw", kind: None},
	{pkg: "image/gif", kind: None},
	{pkg: "image/gif", kind: IO, names: []string{"Decode", "DecodeConfig", "Encode", "EncodeAll"}},
	{pkg: "image/jpeg", kind: None},
	{pkg: "image/jpeg", kind: IO, names: []string{"Decode", "DecodeConfig", "Encode"}},
	{pkg: "image/png", kind: None},
	{pkg: "image/png", kind: IO, names: []string{"Decode", "DecodeConfig", "Encode"}},

	// MIME: parsing pure, multipart streaming IO.
	{pkg: "mime", kind: None},
	{pkg: "mime/quotedprintable", kind: None},
	{pkg: "mime/multipart", kind: None},
	{pkg: "mime/multipart", kind: IO, names: []string{
		"Reader.NextPart", "Reader.NextRawPart", "Reader.ReadForm",
		"Writer.CreatePart", "Writer.WriteField", "Writer.Close",
		"Part.Read", "Part.Close",
	}},

	// Go toolchain packages: pure syntax work. Importing compiled code
	// reads the machine.
	{pkg: "go/ast", kind: None},
	{pkg: "go/build", kind: None},
	{pkg: "go/build", kind: IO, names: []string{"Import", "ImportDir"}},
	{pkg: "go/build/constraint", kind: None},
	{pkg: "go/constant", kind: None},
	{pkg: "go/doc", kind: None},
	{pkg: "go/doc/comment", kind: None},
	{pkg: "go/format", kind: None},
	{pkg: "go/importer", kind: IO},
	{pkg: "go/parser", kind: None},
	{pkg: "go/printer", kind: None},
	{pkg: "go/printer", kind: IO, names: []string{"Fprint"}},
	{pkg: "go/scanner", kind: None},
	{pkg: "go/token", kind: None},
	// go/types values are immutable once checking completes; the package is
	// pure readers over checked type information (including interface
	// dispatch the analyzer normalizes to unexported names like
	// go/types.object.Name). Config.Check is pure computation; the
	// go/importer package it can call into stays IO on its own.
	{pkg: "go/types", kind: None},
	{pkg: "go/version", kind: None},

	// Debug info readers are pure over provided data.
	{pkg: "debug/buildinfo", kind: None},
	{pkg: "debug/dwarf", kind: None},
	{pkg: "debug/elf", kind: None},
	{pkg: "debug/gosym", kind: None},
	{pkg: "debug/macho", kind: None},
	{pkg: "debug/pe", kind: None},
	{pkg: "debug/plan9obj", kind: None},

	// Runtime: mostly pure queries; a few mutate global runtime state.
	{pkg: "runtime", kind: None},
	{pkg: "runtime", kind: Global, names: []string{
		"GC", "GOMAXPROCS", "SetFinalizer", "SetBlockProfileRate",
		"SetMutexProfileFraction",
	}},
	{pkg: "runtime/cgo", kind: None},
	{pkg: "runtime/coverage", kind: None},
	{pkg: "runtime/metrics", kind: None},
	{pkg: "runtime/race", kind: None},
	{pkg: "runtime/debug", kind: None},
	{pkg: "runtime/debug", kind: Global, names: []string{
		"SetGCPercent", "SetMemoryLimit", "SetMaxThreads", "SetMaxStack", "FreeOSMemory",
	}},
	{pkg: "runtime/pprof", kind: None},
	{pkg: "runtime/pprof", kind: IO, names: []string{
		"WriteHeapProfile", "StartCPUProfile", "StopCPUProfile",
	}},
	{pkg: "runtime/trace", kind: None},
	{pkg: "runtime/trace", kind: IO, names: []string{"Start", "Stop"}},

	// Concurrency primitives coordinate but perform no IO.
	{pkg: "sync", kind: None},
	{pkg: "sync/atomic", kind: None},
	{pkg: "context", kind: None},

	// Test helpers.
	{pkg: "testing", kind: None},
	{pkg: "testing/iotest", kind: None},
	{pkg: "testing/fstest", kind: None},
	{pkg: "testing/quick", kind: None},
	{pkg: "testing/slogtest", kind: None},
	{pkg: "testing/synctest", kind: None},
	{pkg: "testing/cryptotest", kind: None},

	// The rest: pure or inert.
	{pkg: "embed", kind: None},
	{pkg: "expvar", kind: Global},
	{pkg: "flag", kind: Global, names: []string{
		"Parse", "NFlag", "NArg", "Arg", "Args", "Lookup", "Set",
		"Visit", "VisitAll", "Var",
	}},
	{pkg: "flag", kind: None, names: []string{"NewFlagSet"}},
	{pkg: "unsafe", kind: Unsafe},
	{pkg: "uuid", kind: None},
	{pkg: "uuid", kind: Random, names: []string{"New"}},
	{pkg: "error", kind: None, names: []string{"Error"}},
}

// stdlibPrefixes builds the classification prefixes from stdlibRules.
func stdlibPrefixes() []Prefix {
	parts := make([][]Prefix, len(stdlibRules))
	for index, rule := range stdlibRules {
		parts[index] = rule.prefixes()
	}
	return slices.Concat(parts...)
}

func (r stdlibRule) prefixes() []Prefix {
	if len(r.names) == 0 {
		return []Prefix{{r.pkg + ".", r.kind}}
	}
	result := make([]Prefix, len(r.names))
	for index, name := range r.names {
		result[index] = Prefix{r.pkg + "." + name, r.kind}
	}
	return result
}
