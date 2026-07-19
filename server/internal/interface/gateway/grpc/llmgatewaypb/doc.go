// Package llmgatewaypb holds the generated Go client/server stubs for the shared
// LLM Gateway gRPC contract defined in server/proto/llmgateway/v1/*.proto
// (CompletionService, ModelsService, and their message types).
//
// These files are generated -- do not hand-edit them. Regenerate with plain
// `protoc` (not `buf generate` -- see "Known discrepancy" below), from
// `server/`:
//
//	protoc -I proto \
//	  --go_out=internal/interface/gateway/grpc/llmgatewaypb \
//	  --go_opt=module=github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway/grpc/llmgatewaypb \
//	  --go-grpc_out=internal/interface/gateway/grpc/llmgatewaypb \
//	  --go-grpc_opt=module=github.com/SHIMA0111/multi-user-ai/server/internal/interface/gateway/grpc/llmgatewaypb \
//	  proto/llmgateway/v1/completion.proto proto/llmgateway/v1/models.proto
//
// This requires protoc-gen-go and protoc-gen-go-grpc on PATH (see buf.gen.yaml's
// header comment for install instructions). The `module=` option strips the
// llmgateway/v1/ path prefix explicitly, keeping the output flat in this
// directory (completion.pb.go, completion_grpc.pb.go, models.pb.go,
// models_grpc.pb.go) rather than mirroring the llmgateway/v1/ proto package
// path.
//
// Known discrepancy (as of Step 34, buf v1.60.0 + protoc-gen-go v1.36.6): running
// `buf generate` with the committed buf.gen.yaml (`paths=source_relative`) nests
// output under `llmgateway/v1/` instead of landing flat here, because buf resolves
// each file's module-relative path to `llmgateway/v1/*.proto` (module root =
// `proto`) and `source_relative` preserves that path verbatim. Until buf.gen.yaml
// is revisited, `buf generate` must not be used -- regenerate with the `protoc`
// invocation above instead.
package llmgatewaypb
