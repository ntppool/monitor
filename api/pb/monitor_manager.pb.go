// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.35.2
// 	protoc        v5.29.3
// source: monitor_manager.proto

package pb

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

type GetConfigParams struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *GetConfigParams) Reset() {
	*x = GetConfigParams{}
	mi := &file_monitor_manager_proto_msgTypes[0]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetConfigParams) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetConfigParams) ProtoMessage() {}

func (x *GetConfigParams) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[0]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetConfigParams.ProtoReflect.Descriptor instead.
func (*GetConfigParams) Descriptor() ([]byte, []int) {
	return file_monitor_manager_proto_rawDescGZIP(), []int{0}
}

type GetServersParams struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *GetServersParams) Reset() {
	*x = GetServersParams{}
	mi := &file_monitor_manager_proto_msgTypes[1]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *GetServersParams) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetServersParams) ProtoMessage() {}

func (x *GetServersParams) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[1]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetServersParams.ProtoReflect.Descriptor instead.
func (*GetServersParams) Descriptor() ([]byte, []int) {
	return file_monitor_manager_proto_rawDescGZIP(), []int{1}
}

// Config is the server set configuration for the monitoring agent
type Config struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Samples    int32       `protobuf:"varint,1,opt,name=samples,proto3" json:"samples,omitempty"`
	IpBytes    []byte      `protobuf:"bytes,2,opt,name=ip_bytes,json=ipBytes,proto3" json:"ip_bytes,omitempty"`
	IpNatBytes []byte      `protobuf:"bytes,3,opt,name=ip_nat_bytes,json=ipNatBytes,proto3" json:"ip_nat_bytes,omitempty"`
	BaseChecks [][]byte    `protobuf:"bytes,4,rep,name=base_checks,json=baseChecks,proto3" json:"base_checks,omitempty"`
	MqttConfig *MQTTConfig `protobuf:"bytes,5,opt,name=mqtt_config,json=mqttConfig,proto3" json:"mqtt_config,omitempty"`
}

func (x *Config) Reset() {
	*x = Config{}
	mi := &file_monitor_manager_proto_msgTypes[2]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Config) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Config) ProtoMessage() {}

func (x *Config) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[2]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Config.ProtoReflect.Descriptor instead.
func (*Config) Descriptor() ([]byte, []int) {
	return file_monitor_manager_proto_rawDescGZIP(), []int{2}
}

func (x *Config) GetSamples() int32 {
	if x != nil {
		return x.Samples
	}
	return 0
}

func (x *Config) GetIpBytes() []byte {
	if x != nil {
		return x.IpBytes
	}
	return nil
}

func (x *Config) GetIpNatBytes() []byte {
	if x != nil {
		return x.IpNatBytes
	}
	return nil
}

func (x *Config) GetBaseChecks() [][]byte {
	if x != nil {
		return x.BaseChecks
	}
	return nil
}

func (x *Config) GetMqttConfig() *MQTTConfig {
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
	mi := &file_monitor_manager_proto_msgTypes[3]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *MQTTConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*MQTTConfig) ProtoMessage() {}

func (x *MQTTConfig) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[3]
	if x != nil {
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
	return file_monitor_manager_proto_rawDescGZIP(), []int{3}
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
	mi := &file_monitor_manager_proto_msgTypes[4]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *Server) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Server) ProtoMessage() {}

func (x *Server) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[4]
	if x != nil {
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
	return file_monitor_manager_proto_rawDescGZIP(), []int{4}
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

type ServerList struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Config  *Config   `protobuf:"bytes,1,opt,name=config,proto3" json:"config,omitempty"`
	Servers []*Server `protobuf:"bytes,2,rep,name=servers,proto3" json:"servers,omitempty"`
	BatchId []byte    `protobuf:"bytes,3,opt,name=batch_id,json=batchId,proto3" json:"batch_id,omitempty"` // todo future api: move this first
}

func (x *ServerList) Reset() {
	*x = ServerList{}
	mi := &file_monitor_manager_proto_msgTypes[5]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServerList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerList) ProtoMessage() {}

func (x *ServerList) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[5]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerList.ProtoReflect.Descriptor instead.
func (*ServerList) Descriptor() ([]byte, []int) {
	return file_monitor_manager_proto_rawDescGZIP(), []int{5}
}

func (x *ServerList) GetConfig() *Config {
	if x != nil {
		return x.Config
	}
	return nil
}

func (x *ServerList) GetServers() []*Server {
	if x != nil {
		return x.Servers
	}
	return nil
}

func (x *ServerList) GetBatchId() []byte {
	if x != nil {
		return x.BatchId
	}
	return nil
}

type ServerStatusList struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Version int32           `protobuf:"varint,1,opt,name=version,proto3" json:"version,omitempty"`
	List    []*ServerStatus `protobuf:"bytes,2,rep,name=List,proto3" json:"List,omitempty"`
	BatchId []byte          `protobuf:"bytes,3,opt,name=batch_id,json=batchId,proto3" json:"batch_id,omitempty"`
}

func (x *ServerStatusList) Reset() {
	*x = ServerStatusList{}
	mi := &file_monitor_manager_proto_msgTypes[6]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServerStatusList) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerStatusList) ProtoMessage() {}

func (x *ServerStatusList) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[6]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerStatusList.ProtoReflect.Descriptor instead.
func (*ServerStatusList) Descriptor() ([]byte, []int) {
	return file_monitor_manager_proto_rawDescGZIP(), []int{6}
}

func (x *ServerStatusList) GetVersion() int32 {
	if x != nil {
		return x.Version
	}
	return 0
}

func (x *ServerStatusList) GetList() []*ServerStatus {
	if x != nil {
		return x.List
	}
	return nil
}

func (x *ServerStatusList) GetBatchId() []byte {
	if x != nil {
		return x.BatchId
	}
	return nil
}

type ServerStatusResult struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ok bool `protobuf:"varint,1,opt,name=ok,proto3" json:"ok,omitempty"`
}

func (x *ServerStatusResult) Reset() {
	*x = ServerStatusResult{}
	mi := &file_monitor_manager_proto_msgTypes[7]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServerStatusResult) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerStatusResult) ProtoMessage() {}

func (x *ServerStatusResult) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[7]
	if x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ServerStatusResult.ProtoReflect.Descriptor instead.
func (*ServerStatusResult) Descriptor() ([]byte, []int) {
	return file_monitor_manager_proto_rawDescGZIP(), []int{7}
}

func (x *ServerStatusResult) GetOk() bool {
	if x != nil {
		return x.Ok
	}
	return false
}

type ServerStatus struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Ticket     []byte                 `protobuf:"bytes,1,opt,name=ticket,proto3" json:"ticket,omitempty"`
	IpBytes    []byte                 `protobuf:"bytes,2,opt,name=ip_bytes,json=ipBytes,proto3" json:"ip_bytes,omitempty"`
	Ts         *timestamppb.Timestamp `protobuf:"bytes,3,opt,name=ts,proto3" json:"ts,omitempty"`
	Offset     *durationpb.Duration   `protobuf:"bytes,4,opt,name=offset,proto3" json:"offset,omitempty"`
	Rtt        *durationpb.Duration   `protobuf:"bytes,5,opt,name=rtt,proto3" json:"rtt,omitempty"`
	Stratum    int32                  `protobuf:"varint,6,opt,name=stratum,proto3" json:"stratum,omitempty"`
	Leap       int32                  `protobuf:"zigzag32,7,opt,name=leap,proto3" json:"leap,omitempty"`
	Error      string                 `protobuf:"bytes,8,opt,name=error,proto3" json:"error,omitempty"`
	NoResponse bool                   `protobuf:"varint,9,opt,name=no_response,json=noResponse,proto3" json:"no_response,omitempty"`
}

func (x *ServerStatus) Reset() {
	*x = ServerStatus{}
	mi := &file_monitor_manager_proto_msgTypes[8]
	ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
	ms.StoreMessageInfo(mi)
}

func (x *ServerStatus) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ServerStatus) ProtoMessage() {}

func (x *ServerStatus) ProtoReflect() protoreflect.Message {
	mi := &file_monitor_manager_proto_msgTypes[8]
	if x != nil {
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
	return file_monitor_manager_proto_rawDescGZIP(), []int{8}
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

var File_monitor_manager_proto protoreflect.FileDescriptor

var file_monitor_manager_proto_rawDesc = []byte{
	0x0a, 0x15, 0x6d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x5f, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65,
	0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x03, 0x61, 0x70, 0x69, 0x1a, 0x1f, 0x67, 0x6f,
	0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x74, 0x69,
	0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1e, 0x67,
	0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x64,
	0x75, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x11, 0x0a,
	0x0f, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x50, 0x61, 0x72, 0x61, 0x6d, 0x73,
	0x22, 0x12, 0x0a, 0x10, 0x47, 0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x50, 0x61,
	0x72, 0x61, 0x6d, 0x73, 0x22, 0xb2, 0x01, 0x0a, 0x06, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12,
	0x18, 0x0a, 0x07, 0x73, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x05,
	0x52, 0x07, 0x73, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x73, 0x12, 0x19, 0x0a, 0x08, 0x69, 0x70, 0x5f,
	0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x69, 0x70, 0x42,
	0x79, 0x74, 0x65, 0x73, 0x12, 0x20, 0x0a, 0x0c, 0x69, 0x70, 0x5f, 0x6e, 0x61, 0x74, 0x5f, 0x62,
	0x79, 0x74, 0x65, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x0a, 0x69, 0x70, 0x4e, 0x61,
	0x74, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x1f, 0x0a, 0x0b, 0x62, 0x61, 0x73, 0x65, 0x5f, 0x63,
	0x68, 0x65, 0x63, 0x6b, 0x73, 0x18, 0x04, 0x20, 0x03, 0x28, 0x0c, 0x52, 0x0a, 0x62, 0x61, 0x73,
	0x65, 0x43, 0x68, 0x65, 0x63, 0x6b, 0x73, 0x12, 0x30, 0x0a, 0x0b, 0x6d, 0x71, 0x74, 0x74, 0x5f,
	0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0f, 0x2e, 0x61,
	0x70, 0x69, 0x2e, 0x4d, 0x51, 0x54, 0x54, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x0a, 0x6d,
	0x71, 0x74, 0x74, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x5e, 0x0a, 0x0a, 0x4d, 0x51, 0x54,
	0x54, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x12, 0x0a, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x04, 0x68, 0x6f, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04, 0x70,
	0x6f, 0x72, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x05, 0x52, 0x04, 0x70, 0x6f, 0x72, 0x74, 0x12,
	0x10, 0x0a, 0x03, 0x6a, 0x77, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x03, 0x6a, 0x77,
	0x74, 0x12, 0x16, 0x0a, 0x06, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x18, 0x04, 0x20, 0x01, 0x28,
	0x0c, 0x52, 0x06, 0x70, 0x72, 0x65, 0x66, 0x69, 0x78, 0x22, 0x51, 0x0a, 0x06, 0x53, 0x65, 0x72,
	0x76, 0x65, 0x72, 0x12, 0x19, 0x0a, 0x08, 0x69, 0x70, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x69, 0x70, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x16,
	0x0a, 0x06, 0x74, 0x69, 0x63, 0x6b, 0x65, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06,
	0x74, 0x69, 0x63, 0x6b, 0x65, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x72, 0x61, 0x63, 0x65, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x05, 0x74, 0x72, 0x61, 0x63, 0x65, 0x22, 0x73, 0x0a, 0x0a,
	0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x23, 0x0a, 0x06, 0x63, 0x6f,
	0x6e, 0x66, 0x69, 0x67, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0b, 0x2e, 0x61, 0x70, 0x69,
	0x2e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x06, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12,
	0x25, 0x0a, 0x07, 0x73, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b,
	0x32, 0x0b, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x52, 0x07, 0x73,
	0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x12, 0x19, 0x0a, 0x08, 0x62, 0x61, 0x74, 0x63, 0x68, 0x5f,
	0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49,
	0x64, 0x22, 0x6e, 0x0a, 0x10, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x12,
	0x25, 0x0a, 0x04, 0x4c, 0x69, 0x73, 0x74, 0x18, 0x02, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x11, 0x2e,
	0x61, 0x70, 0x69, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73,
	0x52, 0x04, 0x4c, 0x69, 0x73, 0x74, 0x12, 0x19, 0x0a, 0x08, 0x62, 0x61, 0x74, 0x63, 0x68, 0x5f,
	0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x62, 0x61, 0x74, 0x63, 0x68, 0x49,
	0x64, 0x22, 0x24, 0x0a, 0x12, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75,
	0x73, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x0e, 0x0a, 0x02, 0x6f, 0x6b, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x08, 0x52, 0x02, 0x6f, 0x6b, 0x22, 0xb2, 0x02, 0x0a, 0x0c, 0x53, 0x65, 0x72, 0x76,
	0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x74, 0x69, 0x63, 0x6b,
	0x65, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x74, 0x69, 0x63, 0x6b, 0x65, 0x74,
	0x12, 0x19, 0x0a, 0x08, 0x69, 0x70, 0x5f, 0x62, 0x79, 0x74, 0x65, 0x73, 0x18, 0x02, 0x20, 0x01,
	0x28, 0x0c, 0x52, 0x07, 0x69, 0x70, 0x42, 0x79, 0x74, 0x65, 0x73, 0x12, 0x2a, 0x0a, 0x02, 0x74,
	0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1a, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x54, 0x69, 0x6d, 0x65, 0x73, 0x74,
	0x61, 0x6d, 0x70, 0x52, 0x02, 0x74, 0x73, 0x12, 0x31, 0x0a, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65,
	0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x12, 0x2b, 0x0a, 0x03, 0x72, 0x74,
	0x74, 0x18, 0x05, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x19, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x44, 0x75, 0x72, 0x61, 0x74, 0x69,
	0x6f, 0x6e, 0x52, 0x03, 0x72, 0x74, 0x74, 0x12, 0x18, 0x0a, 0x07, 0x73, 0x74, 0x72, 0x61, 0x74,
	0x75, 0x6d, 0x18, 0x06, 0x20, 0x01, 0x28, 0x05, 0x52, 0x07, 0x73, 0x74, 0x72, 0x61, 0x74, 0x75,
	0x6d, 0x12, 0x12, 0x0a, 0x04, 0x6c, 0x65, 0x61, 0x70, 0x18, 0x07, 0x20, 0x01, 0x28, 0x11, 0x52,
	0x04, 0x6c, 0x65, 0x61, 0x70, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x08,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x12, 0x1f, 0x0a, 0x0b, 0x6e,
	0x6f, 0x5f, 0x72, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x18, 0x09, 0x20, 0x01, 0x28, 0x08,
	0x52, 0x0a, 0x6e, 0x6f, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x32, 0xb6, 0x01, 0x0a,
	0x07, 0x4d, 0x6f, 0x6e, 0x69, 0x74, 0x6f, 0x72, 0x12, 0x30, 0x0a, 0x09, 0x47, 0x65, 0x74, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x12, 0x14, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x47, 0x65, 0x74, 0x43,
	0x6f, 0x6e, 0x66, 0x69, 0x67, 0x50, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x1a, 0x0b, 0x2e, 0x61, 0x70,
	0x69, 0x2e, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x22, 0x00, 0x12, 0x36, 0x0a, 0x0a, 0x47, 0x65,
	0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x12, 0x15, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x47,
	0x65, 0x74, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x73, 0x50, 0x61, 0x72, 0x61, 0x6d, 0x73, 0x1a,
	0x0f, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x4c, 0x69, 0x73, 0x74,
	0x22, 0x00, 0x12, 0x41, 0x0a, 0x0d, 0x53, 0x75, 0x62, 0x6d, 0x69, 0x74, 0x52, 0x65, 0x73, 0x75,
	0x6c, 0x74, 0x73, 0x12, 0x15, 0x2e, 0x61, 0x70, 0x69, 0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72,
	0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x4c, 0x69, 0x73, 0x74, 0x1a, 0x17, 0x2e, 0x61, 0x70, 0x69,
	0x2e, 0x53, 0x65, 0x72, 0x76, 0x65, 0x72, 0x53, 0x74, 0x61, 0x74, 0x75, 0x73, 0x52, 0x65, 0x73,
	0x75, 0x6c, 0x74, 0x22, 0x00, 0x42, 0x06, 0x5a, 0x04, 0x2e, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_monitor_manager_proto_rawDescOnce sync.Once
	file_monitor_manager_proto_rawDescData = file_monitor_manager_proto_rawDesc
)

func file_monitor_manager_proto_rawDescGZIP() []byte {
	file_monitor_manager_proto_rawDescOnce.Do(func() {
		file_monitor_manager_proto_rawDescData = protoimpl.X.CompressGZIP(file_monitor_manager_proto_rawDescData)
	})
	return file_monitor_manager_proto_rawDescData
}

var file_monitor_manager_proto_msgTypes = make([]protoimpl.MessageInfo, 9)
var file_monitor_manager_proto_goTypes = []any{
	(*GetConfigParams)(nil),       // 0: api.GetConfigParams
	(*GetServersParams)(nil),      // 1: api.GetServersParams
	(*Config)(nil),                // 2: api.Config
	(*MQTTConfig)(nil),            // 3: api.MQTTConfig
	(*Server)(nil),                // 4: api.Server
	(*ServerList)(nil),            // 5: api.ServerList
	(*ServerStatusList)(nil),      // 6: api.ServerStatusList
	(*ServerStatusResult)(nil),    // 7: api.ServerStatusResult
	(*ServerStatus)(nil),          // 8: api.ServerStatus
	(*timestamppb.Timestamp)(nil), // 9: google.protobuf.Timestamp
	(*durationpb.Duration)(nil),   // 10: google.protobuf.Duration
}
var file_monitor_manager_proto_depIdxs = []int32{
	3,  // 0: api.Config.mqtt_config:type_name -> api.MQTTConfig
	2,  // 1: api.ServerList.config:type_name -> api.Config
	4,  // 2: api.ServerList.servers:type_name -> api.Server
	8,  // 3: api.ServerStatusList.List:type_name -> api.ServerStatus
	9,  // 4: api.ServerStatus.ts:type_name -> google.protobuf.Timestamp
	10, // 5: api.ServerStatus.offset:type_name -> google.protobuf.Duration
	10, // 6: api.ServerStatus.rtt:type_name -> google.protobuf.Duration
	0,  // 7: api.Monitor.GetConfig:input_type -> api.GetConfigParams
	1,  // 8: api.Monitor.GetServers:input_type -> api.GetServersParams
	6,  // 9: api.Monitor.SubmitResults:input_type -> api.ServerStatusList
	2,  // 10: api.Monitor.GetConfig:output_type -> api.Config
	5,  // 11: api.Monitor.GetServers:output_type -> api.ServerList
	7,  // 12: api.Monitor.SubmitResults:output_type -> api.ServerStatusResult
	10, // [10:13] is the sub-list for method output_type
	7,  // [7:10] is the sub-list for method input_type
	7,  // [7:7] is the sub-list for extension type_name
	7,  // [7:7] is the sub-list for extension extendee
	0,  // [0:7] is the sub-list for field type_name
}

func init() { file_monitor_manager_proto_init() }
func file_monitor_manager_proto_init() {
	if File_monitor_manager_proto != nil {
		return
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_monitor_manager_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   9,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_monitor_manager_proto_goTypes,
		DependencyIndexes: file_monitor_manager_proto_depIdxs,
		MessageInfos:      file_monitor_manager_proto_msgTypes,
	}.Build()
	File_monitor_manager_proto = out.File
	file_monitor_manager_proto_rawDesc = nil
	file_monitor_manager_proto_goTypes = nil
	file_monitor_manager_proto_depIdxs = nil
}
