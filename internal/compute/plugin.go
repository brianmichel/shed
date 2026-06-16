package compute

import (
	"context"
	"encoding/json"
	"io"

	hplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

const (
	PluginMapKey     = "compute.v1"
	ProtocolVersion  = 1
	MagicCookieKey   = "SHED_COMPUTE_PLUGIN"
	MagicCookieValue = "compute.v1"
)

var Handshake = hplugin.HandshakeConfig{
	ProtocolVersion:  ProtocolVersion,
	MagicCookieKey:   MagicCookieKey,
	MagicCookieValue: MagicCookieValue,
}

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

type jsonCodec struct{}

func (jsonCodec) Name() string                       { return "json" }
func (jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

type HashiPlugin struct {
	hplugin.NetRPCUnsupportedPlugin
	Impl ComputeV1
}

func (p *HashiPlugin) GRPCServer(_ *hplugin.GRPCBroker, s *grpc.Server) error {
	RegisterComputeV1Server(s, &computeV1Server{impl: p.Impl})
	return nil
}

func (p *HashiPlugin) GRPCClient(ctx context.Context, _ *hplugin.GRPCBroker, conn *grpc.ClientConn) (interface{}, error) {
	return &computeV1GRPCClient{ctx: ctx, conn: conn}, nil
}

func PluginSet(impl ComputeV1) hplugin.PluginSet {
	return hplugin.PluginSet{PluginMapKey: &HashiPlugin{Impl: impl}}
}

func ServePlugin(impl ComputeV1) {
	hplugin.Serve(&hplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         PluginSet(impl),
		GRPCServer:      hplugin.DefaultGRPCServer,
	})
}

type InfoRequest struct{}

type computeV1GRPCServerInterface interface {
	Info(context.Context, *InfoRequest) (*PluginInfo, error)
	Allocate(context.Context, *AllocateRequest) (*AllocateResponse, error)
	Status(context.Context, *StatusRequest) (*StatusResponse, error)
	Renew(context.Context, *RenewRequest) (*RenewResponse, error)
	Release(context.Context, *ReleaseRequest) (*ReleaseResponse, error)
	Exec(*ExecRequest, grpc.ServerStream) error
	Stdin(context.Context, *ExecStdinRequest) (*ExecControlResponse, error)
	Cancel(context.Context, *ExecSignalRequest) (*ExecControlResponse, error)
	Kill(context.Context, *ExecSignalRequest) (*ExecControlResponse, error)
}

type computeV1Server struct {
	impl ComputeV1
}

func (s *computeV1Server) Info(ctx context.Context, _ *InfoRequest) (*PluginInfo, error) {
	info, err := s.impl.Info(ctx)
	return &info, err
}
func (s *computeV1Server) Allocate(ctx context.Context, in *AllocateRequest) (*AllocateResponse, error) {
	out, err := s.impl.Allocate(ctx, *in)
	return &out, err
}
func (s *computeV1Server) Status(ctx context.Context, in *StatusRequest) (*StatusResponse, error) {
	out, err := s.impl.Status(ctx, *in)
	return &out, err
}
func (s *computeV1Server) Renew(ctx context.Context, in *RenewRequest) (*RenewResponse, error) {
	out, err := s.impl.Renew(ctx, *in)
	return &out, err
}
func (s *computeV1Server) Release(ctx context.Context, in *ReleaseRequest) (*ReleaseResponse, error) {
	out, err := s.impl.Release(ctx, *in)
	return &out, err
}
func (s *computeV1Server) Exec(in *ExecRequest, stream grpc.ServerStream) error {
	return s.impl.Exec(stream.Context(), *in, func(ev ExecEvent) error {
		return stream.SendMsg(&ev)
	})
}
func (s *computeV1Server) Stdin(ctx context.Context, in *ExecStdinRequest) (*ExecControlResponse, error) {
	out, err := s.impl.Stdin(ctx, *in)
	return &out, err
}
func (s *computeV1Server) Cancel(ctx context.Context, in *ExecSignalRequest) (*ExecControlResponse, error) {
	out, err := s.impl.Cancel(ctx, *in)
	return &out, err
}
func (s *computeV1Server) Kill(ctx context.Context, in *ExecSignalRequest) (*ExecControlResponse, error) {
	out, err := s.impl.Kill(ctx, *in)
	return &out, err
}

func RegisterComputeV1Server(s *grpc.Server, srv computeV1GRPCServerInterface) {
	s.RegisterService(&computeV1ServiceDesc, srv)
}

var computeV1ServiceDesc = grpc.ServiceDesc{
	ServiceName: "shed.compute.v1.Compute",
	HandlerType: (*computeV1GRPCServerInterface)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Info", Handler: infoHandler},
		{MethodName: "Allocate", Handler: allocateHandler},
		{MethodName: "Status", Handler: statusHandler},
		{MethodName: "Renew", Handler: renewHandler},
		{MethodName: "Release", Handler: releaseHandler},
		{MethodName: "Stdin", Handler: stdinHandler},
		{MethodName: "Cancel", Handler: cancelHandler},
		{MethodName: "Kill", Handler: killHandler},
	},
	Streams: []grpc.StreamDesc{
		{StreamName: "Exec", Handler: execHandler, ServerStreams: true},
	},
	Metadata: "shed_compute_v1.json",
}

func infoHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Info(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Info"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Info(ctx, req.(*InfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func allocateHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AllocateRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Allocate(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Allocate"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Allocate(ctx, req.(*AllocateRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func statusHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Status(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Status"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Status(ctx, req.(*StatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func renewHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RenewRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Renew(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Renew"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Renew(ctx, req.(*RenewRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func releaseHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReleaseRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Release(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Release"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Release(ctx, req.(*ReleaseRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func stdinHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExecStdinRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Stdin(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Stdin"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Stdin(ctx, req.(*ExecStdinRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func cancelHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExecSignalRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Cancel(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Cancel"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Cancel(ctx, req.(*ExecSignalRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func killHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ExecSignalRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(computeV1GRPCServerInterface).Kill(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/shed.compute.v1.Compute/Kill"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(computeV1GRPCServerInterface).Kill(ctx, req.(*ExecSignalRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func execHandler(srv interface{}, stream grpc.ServerStream) error {
	in := new(ExecRequest)
	if err := stream.RecvMsg(in); err != nil {
		return err
	}
	return srv.(computeV1GRPCServerInterface).Exec(in, stream)
}

type computeV1GRPCClient struct {
	ctx  context.Context
	conn *grpc.ClientConn
}

func (c *computeV1GRPCClient) Info(ctx context.Context) (PluginInfo, error) {
	out := new(PluginInfo)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Info", &InfoRequest{}, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return PluginInfo{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Allocate(ctx context.Context, in AllocateRequest) (AllocateResponse, error) {
	out := new(AllocateResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Allocate", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return AllocateResponse{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Status(ctx context.Context, in StatusRequest) (StatusResponse, error) {
	out := new(StatusResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Status", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return StatusResponse{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Renew(ctx context.Context, in RenewRequest) (RenewResponse, error) {
	out := new(RenewResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Renew", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return RenewResponse{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Release(ctx context.Context, in ReleaseRequest) (ReleaseResponse, error) {
	out := new(ReleaseResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Release", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return ReleaseResponse{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Exec(ctx context.Context, in ExecRequest, sink ExecEventSink) error {
	stream, err := c.conn.NewStream(ctx, &computeV1ServiceDesc.Streams[0], "/shed.compute.v1.Compute/Exec", grpc.ForceCodec(jsonCodec{}))
	if err != nil {
		return err
	}
	if err := stream.SendMsg(&in); err != nil {
		return err
	}
	if err := stream.CloseSend(); err != nil {
		return err
	}
	for {
		ev := new(ExecEvent)
		err := stream.RecvMsg(ev)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if sink != nil {
			if err := sink(*ev); err != nil {
				return err
			}
		}
	}
}
func (c *computeV1GRPCClient) Stdin(ctx context.Context, in ExecStdinRequest) (ExecControlResponse, error) {
	out := new(ExecControlResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Stdin", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return ExecControlResponse{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Cancel(ctx context.Context, in ExecSignalRequest) (ExecControlResponse, error) {
	out := new(ExecControlResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Cancel", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return ExecControlResponse{}, err
	}
	return *out, nil
}
func (c *computeV1GRPCClient) Kill(ctx context.Context, in ExecSignalRequest) (ExecControlResponse, error) {
	out := new(ExecControlResponse)
	if err := c.conn.Invoke(ctx, "/shed.compute.v1.Compute/Kill", &in, out, grpc.ForceCodec(jsonCodec{})); err != nil {
		return ExecControlResponse{}, err
	}
	return *out, nil
}
