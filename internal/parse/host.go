package parse

import (
	"context"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/sys"
)

// badFileDescriptor is WASI's errno 8.
const badFileDescriptor = 8

// failingHost gives the grammars the functions they import: three answer that nothing is open, and proc_exit ends the
// call, which wazero returns as that call's error. A grammar can reach no file, network, clock or environment.
// tree-sitter calls them only to print a message before an abort.
func failingHost(ctx context.Context, runtime wazero.Runtime) error {
	host := runtime.NewHostModuleBuilder("wasi_snapshot_preview1")
	host.NewFunctionBuilder().WithFunc(func(int32) int32 { return badFileDescriptor }).Export("fd_close")
	host.NewFunctionBuilder().WithFunc(func(int32, int64, int32, int32) int32 { return badFileDescriptor }).Export("fd_seek")
	host.NewFunctionBuilder().WithFunc(func(int32, int32, int32, int32) int32 { return badFileDescriptor }).Export("fd_write")
	host.NewFunctionBuilder().WithFunc(func(code uint32) { panic(sys.NewExitError(code)) }).Export("proc_exit")
	_, err := host.Instantiate(ctx)
	return err
}
