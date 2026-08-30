package application

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
)

const maximumRequestSize = 2 * 1024 * 1024

//go:embed openrpc.json
var openRPCDocument []byte

type ControlServer struct {
	listener net.Listener
	server   *http.Server
	port     int
	sender   func(tea.Msg)
	done     chan struct{}
	close    sync.Once
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *controlError   `json:"error,omitempty"`
}

type controlError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type controlAction uint8

const (
	discoverAction controlAction = iota
	getStateAction
	waitForChangeAction
	getReportAction
	setFocusAction
	selectFileAction
	selectFunctionAction
	scrollSourceAction
	revealLinesAction
	clearLineSelectionAction
	annotateLinesAction
	removeAnnotationAction
	refreshAction
)

type controlCommand struct {
	action        controlAction
	pane          pane
	index         int
	lines         int
	startLine     int
	endLine       int
	message       string
	annotationID  string
	afterRevision uint64
	timeoutMs     int
	reply         chan controlReply
}

type controlReply struct {
	result any
	err    *controlError
}

type methodParser func(json.RawMessage) (controlCommand, *controlError)

var methodParsers = map[string]methodParser{
	"rpc.discover":             parseDiscover,
	"cyclo.getState":           parseGetState,
	"cyclo.waitForChange":      parseWaitForChange,
	"cyclo.getReport":          parseGetReport,
	"cyclo.setFocus":           parseSetFocus,
	"cyclo.selectFile":         parseSelectFile,
	"cyclo.selectFunction":     parseSelectFunction,
	"cyclo.scrollSource":       parseScrollSource,
	"cyclo.revealLines":        parseRevealLines,
	"cyclo.clearLineSelection": parseClearLineSelection,
	"cyclo.annotateLines":      parseAnnotateLines,
	"cyclo.removeAnnotation":   parseRemoveAnnotation,
	"cyclo.refresh":            parseRefresh,
}

func NewControlServer(port int) (*ControlServer, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("control port must be between 0 and 65535")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return nil, fmt.Errorf("bind control port %d: %w", port, err)
	}
	address := listener.Addr().(*net.TCPAddr)
	control := &ControlServer{listener: listener, port: address.Port, done: make(chan struct{})}
	control.server = &http.Server{Handler: http.HandlerFunc(control.handle)}
	return control, nil
}

func (s *ControlServer) Port() int {
	return s.port
}

func (s *ControlServer) URL() string {
	return "http://127.0.0.1:" + strconv.Itoa(s.port)
}

func (s *ControlServer) Start(sender func(tea.Msg)) {
	s.sender = sender
	go func() {
		_ = s.server.Serve(s.listener)
	}()
}

func (s *ControlServer) Close() error {
	var err error
	s.close.Do(func() {
		close(s.done)
		err = s.server.Close()
	})
	return err
}

func (s *ControlServer) handle(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	rpcRequest, rpcError := readRPCRequest(request.Body)
	if rpcError != nil {
		writeRPCError(writer, nil, rpcError)
		return
	}
	command, rpcError := parseRPCRequest(rpcRequest)
	if rpcError != nil {
		writeRPCRequestError(writer, rpcRequest, rpcError)
		return
	}
	if command.action == discoverAction {
		s.writeDiscovery(writer, rpcRequest)
		return
	}

	s.dispatch(writer, request, rpcRequest, command)
}

func (s *ControlServer) dispatch(writer http.ResponseWriter, request *http.Request, rpcRequest rpcRequest, command controlCommand) {
	if s.sender == nil {
		writeRPCRequestError(writer, rpcRequest, runtimeError("Cyclo is not running"))
		return
	}

	command.reply = make(chan controlReply, 1)
	s.sender(command)
	if len(rpcRequest.ID) == 0 {
		writer.WriteHeader(http.StatusNoContent)
		return
	}

	select {
	case reply := <-command.reply:
		writeRPCReply(writer, rpcRequest.ID, reply)
	case <-request.Context().Done():
	case <-s.done:
		writeRPCError(writer, rpcRequest.ID, runtimeError("Cyclo is shutting down"))
	}
}

func (s *ControlServer) writeDiscovery(writer http.ResponseWriter, request rpcRequest) {
	if len(request.ID) == 0 {
		writer.WriteHeader(http.StatusNoContent)
		return
	}

	document, err := discoveryDocument(s.port)
	if err != nil {
		writeRPCError(writer, request.ID, runtimeError(err.Error()))
		return
	}
	writeRPCReply(writer, request.ID, controlReply{result: document})
}

func readRPCRequest(reader io.Reader) (rpcRequest, *controlError) {
	body, err := io.ReadAll(io.LimitReader(reader, maximumRequestSize+1))
	if err != nil {
		return rpcRequest{}, parseError("read request: " + err.Error())
	}
	if len(body) > maximumRequestSize {
		return rpcRequest{}, invalidRequest("request body is too large")
	}
	if !json.Valid(body) {
		return rpcRequest{}, parseError("request body is not valid JSON")
	}

	var request rpcRequest
	err = json.Unmarshal(body, &request)
	if err != nil {
		return rpcRequest{}, invalidRequest("request must be an object")
	}
	return request, nil
}

func parseRPCRequest(request rpcRequest) (controlCommand, *controlError) {
	if request.JSONRPC != "2.0" {
		return controlCommand{}, invalidRequest("jsonrpc must be \"2.0\"")
	}
	if request.Method == "" {
		return controlCommand{}, invalidRequest("method must be a string")
	}

	parser, ok := methodParsers[request.Method]
	if !ok {
		return controlCommand{}, methodNotFound(request.Method)
	}
	return parser(request.Params)
}

func parseDiscover(params json.RawMessage) (controlCommand, *controlError) {
	return parseNoParams(params, discoverAction)
}

func parseGetState(params json.RawMessage) (controlCommand, *controlError) {
	return parseNoParams(params, getStateAction)
}

func parseGetReport(params json.RawMessage) (controlCommand, *controlError) {
	return parseNoParams(params, getReportAction)
}

func parseWaitForChange(params json.RawMessage) (controlCommand, *controlError) {
	values := struct {
		AfterRevision *uint64 `json:"afterRevision"`
		TimeoutMs     *int    `json:"timeoutMs"`
	}{}
	err := decodeNamedParams(params, &values)
	if err != nil {
		return controlCommand{}, err
	}
	if values.AfterRevision == nil {
		return controlCommand{}, invalidParams("afterRevision is required")
	}

	timeoutMs := 30_000
	if values.TimeoutMs != nil {
		timeoutMs = *values.TimeoutMs
	}
	if timeoutMs < 0 || timeoutMs > 60_000 {
		return controlCommand{}, invalidParams("timeoutMs must be between 0 and 60000")
	}
	return controlCommand{
		action:        waitForChangeAction,
		afterRevision: *values.AfterRevision,
		timeoutMs:     timeoutMs,
	}, nil
}

func parseRefresh(params json.RawMessage) (controlCommand, *controlError) {
	return parseNoParams(params, refreshAction)
}

func parseNoParams(params json.RawMessage, action controlAction) (controlCommand, *controlError) {
	err := decodeNamedParams(params, &struct{}{})
	if err != nil {
		return controlCommand{}, err
	}
	return controlCommand{action: action}, nil
}

func parseSetFocus(params json.RawMessage) (controlCommand, *controlError) {
	values := struct {
		Pane *string `json:"pane"`
	}{}
	err := decodeNamedParams(params, &values)
	if err != nil {
		return controlCommand{}, err
	}
	if values.Pane == nil {
		return controlCommand{}, invalidParams("pane is required")
	}

	focus, ok := parsePane(*values.Pane)
	if !ok {
		return controlCommand{}, invalidParams("pane must be files, functions, or source")
	}
	return controlCommand{action: setFocusAction, pane: focus}, nil
}

func parseSelectFile(params json.RawMessage) (controlCommand, *controlError) {
	index, err := parseIndex(params)
	return controlCommand{action: selectFileAction, index: index}, err
}

func parseSelectFunction(params json.RawMessage) (controlCommand, *controlError) {
	index, err := parseIndex(params)
	return controlCommand{action: selectFunctionAction, index: index}, err
}

func parseIndex(params json.RawMessage) (int, *controlError) {
	values := struct {
		Index *int `json:"index"`
	}{}
	err := decodeNamedParams(params, &values)
	if err != nil {
		return 0, err
	}
	if values.Index == nil || *values.Index < 0 {
		return 0, invalidParams("index must be a non-negative integer")
	}
	return *values.Index, nil
}

func parseScrollSource(params json.RawMessage) (controlCommand, *controlError) {
	values := struct {
		Lines *int `json:"lines"`
	}{}
	err := decodeNamedParams(params, &values)
	if err != nil {
		return controlCommand{}, err
	}
	if values.Lines == nil {
		return controlCommand{}, invalidParams("lines is required")
	}
	return controlCommand{action: scrollSourceAction, lines: *values.Lines}, nil
}

func parseRevealLines(params json.RawMessage) (controlCommand, *controlError) {
	startLine, endLine, _, err := parseAnnotationParams(params, false)
	return controlCommand{action: revealLinesAction, startLine: startLine, endLine: endLine}, err
}

func parseClearLineSelection(params json.RawMessage) (controlCommand, *controlError) {
	return parseNoParams(params, clearLineSelectionAction)
}

func parseAnnotateLines(params json.RawMessage) (controlCommand, *controlError) {
	startLine, endLine, message, err := parseAnnotationParams(params, true)
	return controlCommand{
		action: annotateLinesAction, startLine: startLine, endLine: endLine, message: message,
	}, err
}

func parseAnnotationParams(params json.RawMessage, messageRequired bool) (int, int, string, *controlError) {
	values := struct {
		StartLine *int    `json:"startLine"`
		EndLine   *int    `json:"endLine"`
		Message   *string `json:"message"`
	}{}
	if err := decodeNamedParams(params, &values); err != nil {
		return 0, 0, "", err
	}
	startLine, endLine, rangeError := parsedSourceRange(values.StartLine, values.EndLine)
	if rangeError != nil {
		return 0, 0, "", rangeError
	}
	message, messageError := parsedAnnotationMessage(values.Message, messageRequired)
	return startLine, endLine, message, messageError
}

func parsedSourceRange(startLine *int, endLine *int) (int, int, *controlError) {
	if startLine == nil || *startLine < 1 {
		return 0, 0, invalidParams("startLine must be a positive integer")
	}
	end := *startLine
	if endLine != nil {
		end = *endLine
	}
	if end < *startLine {
		return 0, 0, invalidParams("endLine must be greater than or equal to startLine")
	}
	return *startLine, end, nil
}

func parsedAnnotationMessage(value *string, required bool) (string, *controlError) {
	if value == nil {
		if required {
			return "", invalidParams("message must contain between 1 and 160 characters")
		}
		return "", nil
	}
	message := strings.TrimSpace(*value)
	if message == "" || len([]rune(message)) > maximumAnnotationLength {
		return "", invalidParams("message must contain between 1 and 160 characters")
	}
	return message, nil
}

func parseRemoveAnnotation(params json.RawMessage) (controlCommand, *controlError) {
	values := struct {
		AnnotationID *string `json:"annotationId"`
	}{}
	if err := decodeNamedParams(params, &values); err != nil {
		return controlCommand{}, err
	}
	if values.AnnotationID == nil || strings.TrimSpace(*values.AnnotationID) == "" {
		return controlCommand{}, invalidParams("annotationId is required")
	}
	return controlCommand{action: removeAnnotationAction, annotationID: *values.AnnotationID}, nil
}

func decodeNamedParams(params json.RawMessage, value any) *controlError {
	params = bytes.TrimSpace(params)
	if len(params) == 0 || bytes.Equal(params, []byte("null")) {
		params = []byte("{}")
	}
	if params[0] != '{' {
		return invalidParams("params must be an object")
	}

	decoder := json.NewDecoder(bytes.NewReader(params))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(value)
	if err != nil {
		return invalidParams(err.Error())
	}
	return nil
}

func discoveryDocument(port int) (map[string]any, error) {
	var document map[string]any
	err := json.Unmarshal(openRPCDocument, &document)
	if err != nil {
		return nil, fmt.Errorf("decode OpenRPC document: %w", err)
	}
	servers, ok := document["servers"].([]any)
	if !ok || len(servers) == 0 {
		return nil, errors.New("OpenRPC document has no server")
	}
	server, ok := servers[0].(map[string]any)
	if !ok {
		return nil, errors.New("OpenRPC server is invalid")
	}
	server["url"] = "http://127.0.0.1:" + strconv.Itoa(port)
	return document, nil
}

func writeRPCRequestError(writer http.ResponseWriter, request rpcRequest, err *controlError) {
	if len(request.ID) == 0 {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	writeRPCError(writer, request.ID, err)
}

func writeRPCError(writer http.ResponseWriter, id json.RawMessage, err *controlError) {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	writeJSON(writer, rpcResponse{JSONRPC: "2.0", ID: id, Error: err})
}

func writeRPCReply(writer http.ResponseWriter, id json.RawMessage, reply controlReply) {
	if reply.err != nil {
		writeRPCError(writer, id, reply.err)
		return
	}
	writeJSON(writer, rpcResponse{JSONRPC: "2.0", ID: id, Result: reply.result})
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func parseError(message string) *controlError {
	return &controlError{Code: -32700, Message: message}
}

func invalidRequest(message string) *controlError {
	return &controlError{Code: -32600, Message: message}
}

func methodNotFound(method string) *controlError {
	return &controlError{Code: -32601, Message: "method not found: " + method}
}

func invalidParams(message string) *controlError {
	return &controlError{Code: -32602, Message: message}
}

func runtimeError(message string) *controlError {
	return &controlError{Code: -32000, Message: strings.TrimSpace(message)}
}
