package cli

import (
	"context"
	"fmt"

	"github.com/hanthor/oramalama/internal/runtime"
)

type showCmd struct{ r *Runner }

func (c *showCmd) Name() string      { return "show" }
func (c *showCmd) Aliases() []string { return nil }

func (c *showCmd) Run(ctx context.Context, args []string) error {
	model, err := runtime.ResolveShowModel(ctx, "", args)
	if err != nil {
		return err
	}

	info, err := runtime.InspectModel(ctx, model)
	if err != nil {
		return err
	}

	models, err := runtime.InstalledModels(ctx)
	if err != nil {
		return err
	}
	listed, ok := runtime.FindModel(models, model)
	if !ok {
		listed = runtime.ModelInfo{Name: model}
	}

	endpoint := runtime.Endpoint(ctx)
	runningModel, _ := runtime.ModelIDFromEndpoint(ctx, endpoint)

	fmt.Fprintf(c.r.Stdout, "Model:        %s\n", runtime.FirstNonEmpty(listed.Name, model))
	if info.Registry != "" {
		fmt.Fprintf(c.r.Stdout, "Registry:     %s\n", info.Registry)
	}
	if info.Format != "" {
		fmt.Fprintf(c.r.Stdout, "Format:       %s\n", info.Format)
	}
	if arch := runtime.InspectField(ctx, model, "general.architecture"); arch != "" {
		fmt.Fprintf(c.r.Stdout, "Architecture: %s\n", arch)
	}
	if sizeLabel := runtime.InspectField(ctx, model, "general.size_label"); sizeLabel != "" {
		fmt.Fprintf(c.r.Stdout, "Size label:   %s\n", sizeLabel)
	}
	if quant := runtime.QuantFromModelName(runtime.FirstNonEmpty(listed.Name, model)); quant != "" {
		fmt.Fprintf(c.r.Stdout, "Quant:        %s\n", quant)
	}
	if listed.Size > 0 {
		fmt.Fprintf(c.r.Stdout, "Size:         %.1f GB\n", float64(listed.Size)/1024.0/1024.0/1024.0)
	}
	fmt.Fprintf(c.r.Stdout, "Context:      %d tokens\n", runtime.GetCtxSize(runtime.FirstNonEmpty(listed.Name, model)))
	if license := runtime.InspectField(ctx, model, "general.license"); license != "" {
		fmt.Fprintf(c.r.Stdout, "License:      %s\n", license)
	}
	if info.Path != "" {
		fmt.Fprintf(c.r.Stdout, "Path:         %s\n", info.Path)
	}
	if runningModel != "" && runtime.NormalizeModel(runningModel) == runtime.NormalizeModel(runtime.FirstNonEmpty(listed.Name, model)) {
		fmt.Fprintf(c.r.Stdout, "Endpoint:     %s\n", endpoint)
	}

	return nil
}
