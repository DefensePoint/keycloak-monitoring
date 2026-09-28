package reports

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"io"
	"os"
	"sort"
	"time"

	"github.com/DefensePoint/keycloak-monitoring/internal/logger"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// ChromePDFGenerator implements PDFGenerator using Chrome headless.
type ChromePDFGenerator struct {
	logger       *logger.Logger
	templatePath string
	assetsPath   string
}

// NewChromePDFGenerator creates a new Chrome-based PDF generator.
func NewChromePDFGenerator(log *logger.Logger) *ChromePDFGenerator {
	return &ChromePDFGenerator{
		logger:       log,
		templatePath: "internal/reports/templates/report.html",
		assetsPath:   "assets",
	}
}

// Generate creates a PDF report from the aggregated data using Chrome headless.
func (g *ChromePDFGenerator) Generate(data *Data) ([]byte, error) {
	g.logger.Info("ChromePDFGenerator.Generate called - starting PDF generation")

	data.CoverImagePath = ""
	data.LogoPath = ""

	// Sort AlertList by severity
	severityOrder := map[string]int{
		"critical": 1,
		"error":    2,
		"warning":  3,
		"info":     4,
	}
	sort.Slice(data.AlertList, func(i, j int) bool {
		return severityOrder[data.AlertList[i].Severity] < severityOrder[data.AlertList[j].Severity]
	})

	// Load and parse the HTML template with custom functions
	funcMap := template.FuncMap{
		"divf": func(a, b float64) float64 {
			if b == 0 {
				return 0
			}
			return a / b
		},
	}

	tmpl, err := template.New("report.html").Funcs(funcMap).ParseFiles(g.templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load HTML template: %w", err)
	}

	// Execute template with data
	var htmlBuf bytes.Buffer
	if err := tmpl.Execute(&htmlBuf, data); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	// Create temp directory for Chrome user data
	tmpDir, err := os.MkdirTemp("", "chrome-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			g.logger.Warn("Failed to remove temp directory", logger.Err(err))
		}
	}()

	// Copy image files to temp directory
	coverSrc := g.assetsPath + "/cover.png"
	logoSrc := g.assetsPath + "/logo.png"
	coverDst := tmpDir + "/cover.png"
	logoDst := tmpDir + "/logo.png"

	if err := copyFile(coverSrc, coverDst); err != nil {
		g.logger.Warn("Failed to copy cover image", logger.Err(err))
	} else {
		data.CoverImagePath = "cover.png"
		g.logger.Info("Copied cover image to temp dir")
	}

	if err := copyFile(logoSrc, logoDst); err != nil {
		g.logger.Warn("Failed to copy logo image", logger.Err(err))
	} else {
		data.LogoPath = "logo.png"
		g.logger.Info("Copied logo image to temp dir")
	}

	// Re-execute template with updated image paths
	htmlBuf.Reset()
	if err := tmpl.Execute(&htmlBuf, data); err != nil {
		return nil, fmt.Errorf("failed to re-execute template: %w", err)
	}

	// Write HTML to temp file so Chrome can navigate to it with file:// URL
	htmlFile := tmpDir + "/report.html"
	if err := os.WriteFile(htmlFile, htmlBuf.Bytes(), 0644); err != nil {
		return nil, fmt.Errorf("failed to write HTML file: %w", err)
	}

	// Create crash dumps directory for crashpad handler
	crashDir := tmpDir + "/crashes"
	if err := os.MkdirAll(crashDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create crash directory: %w", err)
	}

	g.logger.Info("Setting up Chrome for PDF generation",
		logger.Str("tmpDir", tmpDir),
		logger.Str("htmlFile", htmlFile),
		logger.Str("crashDir", crashDir),
		logger.Str("chromePath", "/usr/bin/chromium"))

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath("/usr/bin/chromium"),
		chromedp.UserDataDir(tmpDir),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Headless,
		chromedp.DisableGPU,
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("allow-file-access-from-files", true),
		chromedp.Flag("disable-web-security", true),
	}

	g.logger.Info("Creating Chrome allocator context")
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	g.logger.Info("Creating chromedp context with 30s timeout")

	timeoutCtx, timeoutCancel := context.WithTimeout(allocCtx, 30*time.Second)
	defer timeoutCancel()

	ctx, cancel := chromedp.NewContext(timeoutCtx)
	defer cancel()

	g.logger.Info("Starting Chrome PDF generation", logger.Str("html_file", htmlFile))
	var pdfBuf []byte

	fileURL := "file://" + htmlFile
	g.logger.Info("Navigating to HTML file", logger.Str("url", fileURL))

	if err := chromedp.Run(ctx,
		chromedp.Navigate(fileURL),
		chromedp.WaitReady("body"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			g.logger.Info("Printing to PDF")
			var err error

			logoBytes, _ := os.ReadFile(g.assetsPath + "/logo.png")
			logoBase64 := base64.StdEncoding.EncodeToString(logoBytes)

			headerTemplate := fmt.Sprintf(`<div style="width:100%%;height:100%%;position:absolute;top:0;left:0;right:0;margin:0;padding:0;"><div style="position:absolute;top:0;left:0;width:0;height:0;border-top:100px solid #db2833;border-right:100px solid transparent;"></div><img src="data:image/png;base64,%s" style="position:absolute;top:15px;right:20px;height:20px;" /></div>`, logoBase64)

			footerTemplate := fmt.Sprintf(`<div style="font-family:Arial,sans-serif;font-size:9px;color:#666;text-align:center;width:100%%;margin:0 auto;padding:5px 0;">Generated on %s | Page <span class="pageNumber"></span> of <span class="totalPages"></span></div>`, data.GeneratedAt.Format("2006-01-02"))

			pdfBuf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(8.27).
				WithPaperHeight(11.69).
				WithMarginTop(0.5).
				WithMarginBottom(0.4).
				WithMarginLeft(0).
				WithMarginRight(0).
				WithPreferCSSPageSize(true).
				WithDisplayHeaderFooter(true).
				WithHeaderTemplate(headerTemplate).
				WithFooterTemplate(footerTemplate).
				Do(ctx)
			if err != nil {
				g.logger.Error("Failed to print PDF", logger.Err(err))
			} else {
				g.logger.Info("PDF generated successfully", logger.Int("pdf_size", len(pdfBuf)))
			}
			return err
		}),
	); err != nil {
		g.logger.Error("Chrome PDF generation failed", logger.Err(err))
		return nil, fmt.Errorf("failed to generate PDF with chromedp: %w", err)
	}

	g.logger.Info("PDF generation completed successfully", logger.Int("size_bytes", len(pdfBuf)))
	return pdfBuf, nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file %s: %w", src, err)
	}
	defer func() {
		_ = sourceFile.Close()
	}()

	destFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination file %s: %w", dst, err)
	}
	defer func() {
		_ = destFile.Close()
	}()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return destFile.Sync()
}
