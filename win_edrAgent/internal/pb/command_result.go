// Package pb provides a runtime-built CommandResult message for SendCommandResult RPC
// when the main proto has not been regenerated to include it.
package pb

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	commandResultDesc   protoreflect.MessageDescriptor
	commandResultDescOnce sync.Once
)

func getCommandResultDesc() protoreflect.MessageDescriptor {
	commandResultDescOnce.Do(func() {
		fd := &descriptorpb.FileDescriptorProto{
			Name:    proto.String("edr/v1/command_result.proto"),
			Package: proto.String("edr.v1"),
			Dependency: []string{
				"google/protobuf/duration.proto",
				"google/protobuf/timestamp.proto",
			},
			MessageType: []*descriptorpb.DescriptorProto{
				{
					Name: proto.String("CommandResult"),
					Field: []*descriptorpb.FieldDescriptorProto{
						{Name: proto.String("command_id"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						{Name: proto.String("agent_id"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						{Name: proto.String("status"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						{Name: proto.String("output"), Number: proto.Int32(4), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						{Name: proto.String("error"), Number: proto.Int32(5), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
						{Name: proto.String("duration"), Number: proto.Int32(6), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".google.protobuf.Duration")},
						{Name: proto.String("timestamp"), Number: proto.Int32(7), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".google.protobuf.Timestamp")},
					},
				},
			},
		}
		file, err := protodesc.NewFile(fd, protoregistry.GlobalFiles)
		if err != nil {
			panic("pb: build CommandResult descriptor: " + err.Error())
		}
		md := file.Messages().Get(0)
		if md == nil || md.Name() != "CommandResult" {
			panic("pb: CommandResult message not found in descriptor")
		}
		commandResultDesc = md
	})
	return commandResultDesc
}

// NewCommandResultProto builds a proto.Message for SendCommandResult from the given fields.
// Used by the gRPC client when the generated edr.pb.go does not yet include CommandResult (run make proto to get it).
func NewCommandResultProto(commandID, agentID, status, output, errStr string, duration time.Duration, timestamp time.Time) proto.Message {
	desc := getCommandResultDesc()
	msg := dynamicpb.NewMessage(desc)
	msg.Set(desc.Fields().ByName("command_id"), protoreflect.ValueOfString(commandID))
	msg.Set(desc.Fields().ByName("agent_id"), protoreflect.ValueOfString(agentID))
	msg.Set(desc.Fields().ByName("status"), protoreflect.ValueOfString(status))
	msg.Set(desc.Fields().ByName("output"), protoreflect.ValueOfString(output))
	msg.Set(desc.Fields().ByName("error"), protoreflect.ValueOfString(errStr))
	msg.Set(desc.Fields().ByName("duration"), protoreflect.ValueOfMessage(durationpb.New(duration).ProtoReflect()))
	msg.Set(desc.Fields().ByName("timestamp"), protoreflect.ValueOfMessage(timestamppb.New(timestamp).ProtoReflect()))
	return msg
}
