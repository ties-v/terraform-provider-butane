package butane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	butane "github.com/coreos/butane/config"
	"github.com/coreos/butane/config/common"
	"github.com/coreos/vcontext/report"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"gopkg.in/yaml.v3"
)

func dataSourceConfig() *schema.Resource {
	return &schema.Resource{
		Description: "Validate and transpile Butane config to Ignition config.",
		Schema: map[string]*schema.Schema{
			"content": {
				Description: "Butane configuration file.",
				Type:        schema.TypeString,
				Required:    true,
				ValidateFunc: func(val any, key string) (warns []string, errs []error) {
					content := val.(string)

					var t any
					if err := yaml.Unmarshal([]byte(content), &t); err != nil {
						errs = append(errs, err)
					}

					return
				},
			},
			// Property follows the official Butane package.
			// Ref: https://github.com/coreos/butane/blob/55aa746eb0b43099040268ba0c70ae3ac2a19567/internal/main.go#L56
			"files_dir": {
				Description: "Directory to embed the local files. Maps to `--files-dir` option on Butane CLI.",
				Type:        schema.TypeString,
				Optional:    true,
			},
			"ignition": {
				Description: "Result Ignition configuration.",
				Type:        schema.TypeString,
				Computed:    true,
			},
			// Property follows the official Butane package.
			// Ref: https://github.com/coreos/butane/blob/55aa746eb0b43099040268ba0c70ae3ac2a19567/internal/main.go#L50
			"pretty": {
				Description: "Output formatted results. Maps to `--pretty` option on Butane CLI.",
				Default:     false,
				Optional:    true,
				Type:        schema.TypeBool,
			},
			// Property follows the official Butane package.
			// Ref: https://github.com/coreos/butane/blob/55aa746eb0b43099040268ba0c70ae3ac2a19567/internal/main.go#L49
			"strict": {
				Description: "Strictly check the format. Any warning will make transpile fail. Maps to `--strict` option on Butane CLI.",
				Default:     false,
				Optional:    true,
				Type:        schema.TypeBool,
			},
		},
		ReadContext: dataSourceButaneReadContext,
	}
}

func dataSourceButaneReadContext(ctx context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
	var diags diag.Diagnostics
	opts := common.TranslateBytesOptions{}

	content := d.Get("content").(string)
	strict := d.Get("strict").(bool)

	if ok, v := d.Get("pretty").(bool); ok {
		opts.Pretty = v
	}

	if v, ok := d.GetOk("files_dir"); ok {
		opts.FilesDir = v.(string)
	}

	b, rpt, err := butane.TranslateBytes([]byte(content), opts)
    if err != nil {
        diags = append(diags, diag.Diagnostic{
            Severity: diag.Error,
            Summary:  "Failed to transpile Butane config",
            Detail:   formatReport(rpt, err),
        })
        return diags
    }

    // Surface non-fatal warnings from the report
    for _, entry := range rpt.Entries {
        if entry.Kind == report.Warn {
            diags = append(diags, diag.Diagnostic{
                Severity: diag.Warning,
                Summary:  "Butane config warning",
                Detail:   entry.String(),
            })
        }
    }
	// Inspired by official Butane.
	// https://github.com/coreos/butane/blob/55aa746eb0b43099040268ba0c70ae3ac2a19567/internal/main.go#L104-L106
	if strict && len(rpt.Entries) > 0 {
		diags = append(diags, diag.Diagnostic{
                Severity: diag.Error,
                Summary:  "Warning with strict enabled",
                Detail:   "The Butane to Ignition conversion resulted in warnings. Since strict is specified, the conversion failed.\n\n" + rpt.String(),
            })
		return diags
	}

	sum := sha256.Sum256(b)
	d.SetId(hex.EncodeToString(sum[:]))
	d.Set("ignition", string(b))
	return diags
}


func formatReport(rpt report.Report, err error) string {
    var sb strings.Builder
    sb.WriteString("Butane could not translate the config to an Ignition config.\n\n")

    if err != nil {
        sb.WriteString("Error: ")
        sb.WriteString(err.Error())
        sb.WriteString("\n")
    }

    // rpt.String() iterates through entries and calls entry.String() on each,
    // producing output like: "error at $.variant: invalid variant \"foo\""
    reportStr := rpt.String()
    if reportStr != "" {
        sb.WriteString("\nDetailed errors from Butane:\n")
        sb.WriteString(reportStr)
    }

    sb.WriteString("\nCheck your Butane config against the specification: https://coreos.github.io/butane/")
    return sb.String()
}

