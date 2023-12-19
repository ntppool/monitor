// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        (unknown)
// source: api/v1/monitor_manager.proto

package apiv1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	durationpb "google.golang.org/protobuf/types/known/durationpb"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type GetConfigRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *GetConfigRequest) Reset() {
	*x = GetConfigRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetConfigRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetConfigRequest) ProtoMessage() {}

func (x *GetConfigRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetConfigRequest.ProtoReflect.Descriptor instead.
func (*GetConfigRequest) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{0}
}

type GetServersRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *GetServersRequest) Reset() {
	*x = GetServersRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetServersRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetServersRequest) ProtoMessage() {}

func (x *GetServersRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetServersRequest.ProtoReflect.Descriptor instead.
func (*GetServersRequest) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{1}
}

// GetConfigResponse is the server set configuration for the monitoring agent
type GetConfigResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Samples    int32       `protobuf:"varint,1,opt,name=samples,proto3" json:"samples,omitempty"`
	IpBytes    []byte      `protobuf:"bytes,2,opt,name=ip_bytes,json=ipBytes,proto3" json:"ip_bytes,omitempty"`
	IpNatBytes []byte      `protobuf:"bytes,3,opt,name=ip_nat_bytes,json=ipNatBytes,proto3" json:"ip_nat_bytes,omitempty"`
	BaseChecks [][]byte    `protobuf:"bytes,4,rep,name=base_checks,json=baseChecks,proto3" json:"base_checks,omitempty"`
	MqttConfig *MQTTConfig `protobuf:"bytes,5,opt,name=mqtt_config,json=mqttConfig,proto3" json:"mqtt_config,omitempty"`
}

func (x *GetConfigResponse) Reset() {
	*x = GetConfigResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetConfigResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetConfigResponse) ProtoMessage() {}

func (x *GetConfigResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetConfigResponse.ProtoReflect.Descriptor instead.
func (*GetConfigResponse) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{2}
}

func (x *GetConfigResponse) GetSamples() int32 {
	if x != nil {
		return x.Samples
	}
	return 0
}

func (x *GetConfigResponse) GetIpBytes() []byte {
	if x != nil {
		return x.IpBytes
	}
	return nil
}

func (x *GetConfigResponse) GetIpNatBytes() []byte {
	if x != nil {
		return x.IpNatBytes
	}
	return nil
}

func (x *GetConfigResponse) GetBaseChecks() [][]byte {
	if x != nil {
		return x.BaseChecks
	}
	return nil
}

func (x *GetConfigResponse) GetMqttConfig() *MQTTConfig {
	if x != nil {
		return x.MqttConfig
	}
	return nil
}

type MQTTConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Host   []byte `protobuf:"bytes,1,opt,name=host,proto3" json:"host,omitempty"`
	Port   int32  `protobuf:"varint,2,opt,name=port,proto3" json:"port,omitempty"`
	Jwt    []byte `protobuf:"bytes,3,opt,name=jwt,proto3" json:"jwt,omitempty"`
	Prefix []byte `protobuf:"bytes,4,opt,name=prefix,proto3" json:"prefix,omitempty"` // base prefix for topic ("/devel/monitors/" for example)
}

func (x *MQTTConfig) Reset() {
	*x = MQTTConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *MQTTConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MQTTConfig) ProtoMessage() {}

func (x *MQTTConfig) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use MQTTConfig.ProtoReflect.Descriptor instead.
func (*MQTTConfig) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{3}
}

func (x *MQTTConfig) GetHost() []byte {
	if x != nil {
		return x.Host
	}
	return nil
}

func (x *MQTTConfig) GetPort() int32 {
	if x != nil {
		return x.Port
	}
	return 0
}

func (x *MQTTConfig) GetJwt() []byte {
	if x != nil {
		return x.Jwt
	}
	return nil
}

func (x *MQTTConfig) GetPrefix() []byte {
	if x != nil {
		return x.Prefix
	}
	return nil
}

type Server struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	IpBytes []byte `protobuf:"bytes,1,opt,name=ip_bytes,json=ipBytes,proto3" json:"ip_bytes,omitempty"`
	Ticket  []byte `protobuf:"bytes,2,opt,name=ticket,proto3" json:"ticket,omitempty"`
	Trace   bool   `protobuf:"varint,3,opt,name=trace,proto3" json:"trace,omitempty"`
}

func (x *Server) Reset() {
	*x = Server{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Server) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Server) ProtoMessage() {}

func (x *Server) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Server.ProtoReflect.Descriptor instead.
func (*Server) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{4}
}

func (x *Server) GetIpBytes() []byte {
	if x != nil {
		return x.IpBytes
	}
	return nil
}

func (x *Server) GetTicket() []byte {
	if x != nil {
		return x.Ticket
	}
	return nil
}

func (x *Server) GetTrace() bool {
	if x != nil {
		return x.Trace
	}
	return false
}

type GetServersResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Config  *GetConfigResponse `protobuf:"bytes,1,opt,name=config,proto3" json:"config,omitempty"`
	Servers []*Server          `protobuf:"bytes,2,rep,name=servers,proto3" json:"servers,omitempty"`
	BatchId []byte             `protobuf:"bytes,3,opt,name=batch_id,json=batchId,proto3" json:"batch_id,omitempty"` // todo future api: move this first
}

func (x *GetServersResponse) Reset() {
	*x = GetServersResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetServersResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetServersResponse) ProtoMessage() {}

func (x *GetServersResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetServersResponse.ProtoReflect.Descriptor instead.
func (*GetServersResponse) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{5}
}

func (x *GetServersResponse) GetConfig() *GetConfigResponse {
	if x != nil {
		return x.Config
	}
	return nil
}

func (x *GetServersResponse) GetServers() []*Server {
	if x != nil {
		return x.Servers
	}
	return nil
}

func (x *GetServersResponse) GetBatchId() []byte {
	if x != nil {
		return x.BatchId
	}
	return nil
}

type SubmitResultsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Bersion int32           `protobuf:"varint,1,opt,name=bersion,proto3" json:"bersion,omitempty"`
	List    []*ServerStatus `protobuf:"bytes,2,rep,name=list,proto3" json:"list,omitempty"`
	BatchId []byte          `protobuf:"bytes,3,opt,name=batch_id,json=batchId,proto3" json:"batch_id,omitempty"`
}

func (x *SubmitResultsRequest) Reset() {
	*x = SubmitResultsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SubmitResultsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmitResultsRequest) ProtoMessage() {}

func (x *SubmitResultsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmitResultsRequest.ProtoReflect.Descriptor instead.
func (*SubmitResultsRequest) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{6}
}

func (x *SubmitResultsRequest) GetBersion() int32 {
	if x != nil {
		return x.Bersion
	}
	return 0
}

func (x *SubmitResultsRequest) GetList() []*ServerStatus {
	if x != nil {
		return x.List
	}
	return nil
}

func (x *SubmitResultsRequest) GetBatchId() []byte {
	if x != nil {
		return x.BatchId
	}
	return nil
}

type SubmitResultsResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ok bool `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
}

func (x *SubmitResultsResponse) Reset() {
	*x = SubmitResultsResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SubmitResultsResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SubmitResultsResponse) ProtoMessage() {}

func (x *SubmitResultsResponse) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SubmitResultsResponse.ProtoReflect.Descriptor instead.
func (*SubmitResultsResponse) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{7}
}

func (x *SubmitResultsResponse) GetOk() bool {
	if x != nil {
		return x.Ok
	}
	return false
}

type ServerStatus struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ticket           []byte                 `protobuf:"bytes,1,opt,name=ticket,proto3" json:"ticket,omitempty"`
	IpBytes          []byte                 `protobuf:"bytes,2,opt,name=ip_bytes,json=ipBytes,proto3" json:"ip_bytes,omitempty"`
	Ts               *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=ts,proto3" json:"ts,omitempty"`
	Offset           *durationpb.Duration   `protobuf:"bytes,4,opt,name=offset,proto3" json:"offset,omitempty"`
	Rtt              *durationpb.Duration   `protobuf:"bytes,5,opt,name=rtt,proto3" json:"rtt,omitempty"`
	Stratum          int32                  `protobuf:"varint,6,opt,name=stratum,proto3" json:"stratum,omitempty"`
	Leap             int32                  `protobuf:"zigzag32,7,opt,name=leap,proto3" json:"leap,omitempty"`
	Error            string                 `protobuf:"bytes,8,opt,name=error,proto3" json:"error,omitempty"`
	NoResponse       bool                   `protobuf:"varint,9,opt,name=no_response,json=noResponse,proto3" json:"no_response,omitempty"`
	Responses        [][]byte               `protobuf:"bytes,10,rep,name=responses,proto3" json:"responses,omitempty"`
	SelectedResponse int32                  `protobuf:"zigzag32,11,opt,name=selected_response,json=selectedResponse,proto3" json:"selected_response,omitempty"`
}

func (x *ServerStatus) Reset() {
	*x = ServerStatus{}
	if protoimpl.UnsafeEnabled {
		mi := &file_api_v1_monitor_manager_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ServerStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerStatus) ProtoMessage() {}

func (x *ServerStatus) ProtoReflect() protoreflect.Message {
	mi := &file_api_v1_monitor_manager_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerStatus.ProtoReflect.Descriptor instead.
func (*ServerStatus) Descriptor() ([]byte, []int) {
	return file_api_v1_monitor_manager_proto_rawDescGZIP(), []int{8}
}

func (x *ServerStatus) GetTicket() []byte {
	if x != nil {
		return x.Ticket
	}
	return nil
}

func (x *ServerStatus) GetIpBytes() []byte {
	if x != nil {
		return x.IpBytes
	}
	return nil
}

func (x *ServerStatus) GetTs() *timestamppb.Timestamp {
	if x != nil {
		return x.Ts
	}
	return nil
}

func (x *ServerStatus) GetOffset() *durationpb.Duration {
	if x != nil {
		return x.Offset
	}
	return nil
}

func (x *ServerStatus) GetRtt() *durationpb.Duration {
	if x != nil {
		return x.Rtt
	}
	return nil
}

func (x *ServerStatus) GetStratum() int32 {
	if x != nil {
		return x.Stratum
	}
	return 0
}

func (x *ServerStatus) GetLeap() int32 {
	if x != nil {
		return x.Leap
	}
	return 0
}

func (x *ServerStatus) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *ServerStatus) GetNoResponse() bool {
	if x != nil {
		return x.NoResponse
	}
	return false
}

func (x *ServerStatus) GetResponses() [][]byte {
	if x != nil {
		return x.Responses
	}
	return nil
}

func (x *ServerStatus) GetSelectedResponse() int32 {
	if x != nil {
		return x.SelectedResponse
	}
	return 0
}

var File_api_v1_monitor_manager_proto protoreflect.FileDescriptor

var file_api_v1_monitor_manager_proto_rawDesc = []byte{
	0x0a, 0x1c, 0x61, 0x70, 0x69, 0x2f, 0x76, 0x31, 0x2f, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72,
	0x5f, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x06,
	0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x1a, 0x1f, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f,
	0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x12, 0x0a, 0x10, 0x47, 0x65, 0x74, 0x43, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x22, 0x13, 0x0a, 0x11, 0x47,
	0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x22, 0xc0, 0x01, 0x0a, 0x11, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x73, 0x61, 0x6d, 0x70, 0x6c, 0x65,
	0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x73, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x73,
	0x12, 0x19, 0x0a, 0x08, 0x69, 0x70, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x07, 0x69, 0x70, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x20, 0x0a, 0x0c, 0x69,
	0x70, 0x5f, 0x6e, 0x61, 0x74, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x0a, 0x69, 0x70, 0x4e, 0x61, 0x74, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x1f, 0x0a,
	0x0b, 0x62, 0x61, 0x73, 0x65, 0x5f, 0x63, 0x68, 0x65, 0x63, 0x6b, 0x73, 0x18, 0x04, 0x20, 0x03,
	0x28, 0x0c, 0x52, 0x0a, 0x62, 0x61, 0x73, 0x65, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x73, 0x12, 0x33,
	0x0a, 0x0b, 0x6d, 0x71, 0x74, 0x74, 0x5f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x4d, 0x51, 0x54,
	0x54, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x0a, 0x6d, 0x71, 0x74, 0x74, 0x43, 0x6f, 0x6e,
	0x66, 0x69, 0x67, 0x22, 0x5e, 0x0a, 0x0a, 0x4d, 0x51, 0x54, 0x54, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x04, 0x68, 0x6f, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x18, 0x02, 0x20,
	0x01, 0x28, 0x05, 0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x12, 0x10, 0x0a, 0x03, 0x6a, 0x77, 0x74,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6a, 0x77, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x70,
	0x72, 0x65, 0x66, 0x69, 0x78, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x70, 0x72, 0x65,
	0x66, 0x69, 0x78, 0x22, 0x51, 0x0a, 0x06, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x12, 0x19, 0x0a,
	0x08, 0x69, 0x70, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52,
	0x07, 0x69, 0x70, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x74, 0x69, 0x63, 0x6b,
	0x65, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x74, 0x69, 0x63, 0x6b, 0x65, 0x74,
	0x12, 0x14, 0x0a, 0x05, 0x74, 0x72, 0x61, 0x63, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52,
	0x05, 0x74, 0x72, 0x61, 0x63, 0x65, 0x22, 0x8c, 0x01, 0x0a, 0x12, 0x47, 0x65, 0x74, 0x53, 0x65,
	0x72, 0x76, 0x65, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x31, 0x0a,
	0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67,
	0x12, 0x28, 0x0a, 0x07, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x0e, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65,
	0x72, 0x52, 0x07, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x12, 0x19, 0x0a, 0x08, 0x62, 0x61,
	0x74, 0x63, 0x68, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x62, 0x61,
	0x74, 0x63, 0x68, 0x49, 0x64, 0x22, 0x75, 0x0a, 0x14, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x52,
	0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x18, 0x0a,
	0x07, 0x62, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07,
	0x62, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12, 0x28, 0x0a, 0x04, 0x6c, 0x69, 0x73, 0x74, 0x18,
	0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x53,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x04, 0x6c, 0x69, 0x73,
	0x74, 0x12, 0x19, 0x0a, 0x08, 0x62, 0x61, 0x74, 0x63, 0x68, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0c, 0x52, 0x07, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49, 0x64, 0x22, 0x27, 0x0a, 0x15,
	0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x6f, 0x6b, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x08, 0x52, 0x02, 0x6f, 0x6b, 0x22, 0xfd, 0x02, 0x0a, 0x0c, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x74, 0x69, 0x63, 0x6b, 0x65, 0x74,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x74, 0x69, 0x63, 0x6b, 0x65, 0x74, 0x12, 0x19,
	0x0a, 0x08, 0x69, 0x70, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c,
	0x52, 0x07, 0x69, 0x70, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x2a, 0x0a, 0x02, 0x74, 0x73, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d,
	0x70, 0x52, 0x02, 0x74, 0x73, 0x12, 0x31, 0x0a, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x52, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x12, 0x2b, 0x0a, 0x03, 0x72, 0x74, 0x74, 0x18,
	0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x52, 0x03, 0x72, 0x74, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x73, 0x74, 0x72, 0x61, 0x74, 0x75, 0x6d,
	0x18, 0x06, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x73, 0x74, 0x72, 0x61, 0x74, 0x75, 0x6d, 0x12,
	0x12, 0x0a, 0x04, 0x6c, 0x65, 0x61, 0x70, 0x18, 0x07, 0x20, 0x01, 0x28, 0x11, 0x52, 0x04, 0x6c,
	0x65, 0x61, 0x70, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x08, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x12, 0x1f, 0x0a, 0x0b, 0x6e, 0x6f, 0x5f,
	0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28, 0x08, 0x52, 0x0a,
	0x6e, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x1c, 0x0a, 0x09, 0x72, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x73, 0x18, 0x0a, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x09, 0x72,
	0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x73, 0x12, 0x2b, 0x0a, 0x11, 0x73, 0x65, 0x6c, 0x65,
	0x63, 0x74, 0x65, 0x64, 0x5f, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x0b, 0x20,
	0x01, 0x28, 0x11, 0x52, 0x10, 0x73, 0x65, 0x6c, 0x65, 0x63, 0x74, 0x65, 0x64, 0x52, 0x65, 0x73,
	0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0xeb, 0x01, 0x0a, 0x0e, 0x4d, 0x6f, 0x6e, 0x69, 0x74, 0x6f,
	0x72, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x42, 0x0a, 0x09, 0x47, 0x65, 0x74, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x18, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x47,
	0x65, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a,
	0x19, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x45, 0x0a, 0x0a,
	0x47, 0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x12, 0x19, 0x2e, 0x61, 0x70, 0x69,
	0x2e, 0x76, 0x31, 0x2e, 0x47, 0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1a, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x47,
	0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x00, 0x12, 0x4e, 0x0a, 0x0d, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x52, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x73, 0x12, 0x1c, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x75,
	0x62, 0x6d, 0x69, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x1d, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x76, 0x31, 0x2e, 0x53, 0x75, 0x62, 0x6d,
	0x69, 0x74, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x73, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73,
	0x65, 0x22, 0x00, 0x42, 0x83, 0x01, 0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x2e, 0x61, 0x70, 0x69, 0x2e,
	0x76, 0x31, 0x42, 0x13, 0x4d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x4d, 0x61, 0x6e, 0x61, 0x67,
	0x65, 0x72, 0x50, 0x72, 0x6f, 0x74, 0x6f, 0x50, 0x01, 0x5a, 0x27, 0x67, 0x6f, 0x2e, 0x6e, 0x74,
	0x70, 0x70, 0x6f, 0x6f, 0x6c, 0x2e, 0x6f, 0x72, 0x67, 0x2f, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f,
	0x72, 0x2f, 0x67, 0x65, 0x6e, 0x2f, 0x61, 0x70, 0x69, 0x2f, 0x76, 0x31, 0x3b, 0x61, 0x70, 0x69,
	0x76, 0x31, 0xa2, 0x02, 0x03, 0x41, 0x58, 0x58, 0xaa, 0x02, 0x06, 0x41, 0x70, 0x69, 0x2e, 0x56,
	0x31, 0xca, 0x02, 0x06, 0x41, 0x70, 0x69, 0x5c, 0x56, 0x31, 0xe2, 0x02, 0x12, 0x41, 0x70, 0x69,
	0x5c, 0x56, 0x31, 0x5c, 0x47, 0x50, 0x42, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0xea,
	0x02, 0x07, 0x41, 0x70, 0x69, 0x3a, 0x3a, 0x56, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_api_v1_monitor_manager_proto_rawDescOnce sync.Once
	file_api_v1_monitor_manager_proto_rawDescData = file_api_v1_monitor_manager_proto_rawDesc
)

func file_api_v1_monitor_manager_proto_rawDescGZIP() []byte {
	file_api_v1_monitor_manager_proto_rawDescOnce.Do(func() {
		file_api_v1_monitor_manager_proto_rawDescData = protoimpl.X.CompressGZIP(file_api_v1_monitor_manager_proto_rawDescData)
	})
	return file_api_v1_monitor_manager_proto_rawDescData
}

var file_api_v1_monitor_manager_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_api_v1_monitor_manager_proto_goTypes = []interface{}{
	(*GetConfigRequest)(nil),      // 0: api.v1.GetConfigRequest
	(*GetServersRequest)(nil),     // 1: api.v1.GetServersRequest
	(*GetConfigResponse)(nil),     // 2: api.v1.GetConfigResponse
	(*MQTTConfig)(nil),            // 3: api.v1.MQTTConfig
	(*Server)(nil),                // 4: api.v1.Server
	(*GetServersResponse)(nil),    // 5: api.v1.GetServersResponse
	(*SubmitResultsRequest)(nil),  // 6: api.v1.SubmitResultsRequest
	(*SubmitResultsResponse)(nil), // 7: api.v1.SubmitResultsResponse
	(*ServerStatus)(nil),          // 8: api.v1.ServerStatus
	(*timestamppb.Timestamp)(nil), // 9: google.protobuf.Timestamp
	(*durationpb.Duration)(nil),   // 10: google.protobuf.Duration
}
var file_api_v1_monitor_manager_proto_depIdxs = []int32{
	3,  // 0: api.v1.GetConfigResponse.mqtt_config:type_name -> api.v1.MQTTConfig
	2,  // 1: api.v1.GetServersResponse.config:type_name -> api.v1.GetConfigResponse
	4,  // 2: api.v1.GetServersResponse.servers:type_name -> api.v1.Server
	8,  // 3: api.v1.SubmitResultsRequest.list:type_name -> api.v1.ServerStatus
	9,  // 4: api.v1.ServerStatus.ts:type_name -> google.protobuf.Timestamp
	10, // 5: api.v1.ServerStatus.offset:type_name -> google.protobuf.Duration
	10, // 6: api.v1.ServerStatus.rtt:type_name -> google.protobuf.Duration
	0,  // 7: api.v1.MonitorService.GetConfig:input_type -> api.v1.GetConfigRequest
	1,  // 8: api.v1.MonitorService.GetServers:input_type -> api.v1.GetServersRequest
	6,  // 9: api.v1.MonitorService.SubmitResults:input_type -> api.v1.SubmitResultsRequest
	2,  // 10: api.v1.MonitorService.GetConfig:output_type -> api.v1.GetConfigResponse
	5,  // 11: api.v1.MonitorService.GetServers:output_type -> api.v1.GetServersResponse
	7,  // 12: api.v1.MonitorService.SubmitResults:output_type -> api.v1.SubmitResultsResponse
	10, // [10:13] is the sub-list for method output_type
	7,  // [7:10] is the sub-list for method input_type
	7,  // [7:7] is the sub-list for extension type_name
	7,  // [7:7] is the sub-list for extension extendee
	0,  // [0:7] is the sub-list for field type_name
}

func init() { file_api_v1_monitor_manager_proto_init() }
func file_api_v1_monitor_manager_proto_init() {
	if File_api_v1_monitor_manager_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_api_v1_monitor_manager_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetConfigRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetServersRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetConfigResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*MQTTConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Server); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetServersResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SubmitResultsRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SubmitResultsResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_api_v1_monitor_manager_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ServerStatus); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_api_v1_monitor_manager_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_api_v1_monitor_manager_proto_goTypes,
		DependencyIndexes: file_api_v1_monitor_manager_proto_depIdxs,
		MessageInfos:      file_api_v1_monitor_manager_proto_msgTypes,
	}.Build()
	File_api_v1_monitor_manager_proto = out.File
	file_api_v1_monitor_manager_proto_rawDesc = nil
	file_api_v1_monitor_manager_proto_goTypes = nil
	file_api_v1_monitor_manager_proto_depIdxs = nil
}
