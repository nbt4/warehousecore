package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"

	commonbranding "github.com/nbt4/cores-common/pkg/branding"
	commonhealth "github.com/nbt4/cores-common/pkg/health"
	"warehousecore/config"
	"warehousecore/internal/handlers"
	"warehousecore/internal/led"
	"warehousecore/internal/metrics"
	"warehousecore/internal/middleware"
	"warehousecore/internal/models"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

var brandingSvc *services.BrandingService

// spaHandler serves the SPA and falls back to index.html for client-side routes
func spaHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/sw.js" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	if r.URL.Path == "/manifest.webmanifest" {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
		if brandingSvc != nil {
			_ = json.NewEncoder(w).Encode(commonbranding.Manifest(brandingSvc.GetConfig(), commonbranding.ManifestOptions{
				Name: "WarehouseCore", StartURL: "/", Scope: "/",
				FallbackIcon192: "/app-icons/icon-192.png", FallbackIcon512: "/app-icons/icon-512.png",
				FallbackMaskable: "/app-icons/icon-maskable-512.png",
			}))
			return
		}
	}

	// Build file path
	path := "./web/dist" + r.URL.Path

	// Check if file exists
	fileInfo, err := os.Stat(path)
	if os.IsNotExist(err) {
		// File doesn't exist - serve index.html with injected config
		serveIndexWithConfig(w, r)
		return
	}

	// If path is a directory (not a file), serve index.html for SPA routing
	if err == nil && fileInfo.IsDir() {
		serveIndexWithConfig(w, r)
		return
	}

	// File exists and is not a directory - serve it
	http.FileServer(http.Dir("./web/dist")).ServeHTTP(w, r)
}

// brandingLogoHandler prefers centrally managed branding files and falls back
// to the application logos bundled into the frontend image.
func brandingLogoHandler(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Path)
	for _, directory := range []string{"/var/lib/branding/logos", "./web/dist/logos"} {
		candidate := filepath.Join(directory, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			http.ServeFile(w, r, candidate)
			return
		}
	}
	http.NotFound(w, r)
}

// serveIndexWithConfig injects runtime configuration into index.html
func serveIndexWithConfig(w http.ResponseWriter, r *http.Request) {
	// Read the index.html file
	file, err := os.Open("./web/dist/index.html")
	if err != nil {
		http.Error(w, "Could not open index.html", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Could not read index.html", http.StatusInternalServerError)
		return
	}

	// Get domain configuration from environment variables
	// These should be just the domain (e.g., "rent.server-nt.de")
	// without protocol or port - the frontend will add the protocol
	rentalCoreDomain := os.Getenv("RENTALCORE_DOMAIN")
	warehouseCoreDomain := os.Getenv("WAREHOUSECORE_DOMAIN")

	// Resolve company branding
	companyName := "WarehouseCore"
	if brandingSvc != nil {
		cfg := brandingSvc.GetConfig()
		companyName = cfg.CompanyName
		// Build full config script
		configScript := fmt.Sprintf(
			`<script>window.__APP_CONFIG__={rentalCoreDomain:"%s",warehouseCoreDomain:"%s",companyName:"%s",branding:{companyName:"%s",brandName:"%s",sidebarLogo:"%s",loginLogo:"%s",favicon:"%s",logoSizeSidebar:%d,logoSizeLogin:%d}};</script>`,
			template.JSEscapeString(rentalCoreDomain),
			template.JSEscapeString(warehouseCoreDomain),
			template.JSEscapeString(companyName),
			template.JSEscapeString(cfg.CompanyName),
			template.JSEscapeString(cfg.BrandName),
			template.JSEscapeString(cfg.LogoSidebar),
			template.JSEscapeString(cfg.LogoLogin),
			template.JSEscapeString(cfg.FaviconPath),
			cfg.LogoSizeSidebar,
			cfg.LogoSizeLogin,
		)
		var appleIcon string
		if cfg.FaviconPath != "" {
			appleIcon = fmt.Sprintf(`<link rel="apple-touch-icon" sizes="180x180" href="%s">`, template.HTMLEscapeString(cfg.FaviconPath))
		}
		modifiedContent := bytes.Replace(content, []byte("</head>"), []byte(configScript+appleIcon+"</head>"), 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(modifiedContent)
		return
	}

	// Fallback: old format
	configScript := fmt.Sprintf(
		`<script>window.__APP_CONFIG__={rentalCoreDomain:"%s",warehouseCoreDomain:"%s",companyName:"%s"};</script>`,
		template.JSEscapeString(rentalCoreDomain),
		template.JSEscapeString(warehouseCoreDomain),
		template.JSEscapeString(companyName),
	)

	// Inject the script before </head>
	modifiedContent := bytes.Replace(content, []byte("</head>"), []byte(configScript+"</head>"), 1)

	// Serve the modified content
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(modifiedContent)
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	if err := repository.InitDatabase(cfg); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer repository.CloseDatabase()
	if err := handlers.EnsureCableInventorySchema(); err != nil {
		log.Fatalf("Failed to initialize cable inventory schema: %v", err)
	}
	if err := handlers.EnsureProductManagementSchema(); err != nil {
		log.Fatalf("Failed to initialize product management schema: %v", err)
	}
	if err := handlers.EnsureDeviceStatusSchema(); err != nil {
		log.Fatalf("Failed to initialize device status schema: %v", err)
	}
	if err := handlers.EnsureLabelStudioSchema(); err != nil {
		log.Fatalf("Failed to initialize label studio schema: %v", err)
	}

	// Set initial DB connection gauge
	if sqlDB := repository.GetSQLDB(); sqlDB != nil {
		metrics.DBConnectionsOpen.Set(float64(sqlDB.Stats().OpenConnections))
	}

	brandingSvc = services.NewBrandingService(repository.GetDB(), "warehouse")

	// Initialize LED service
	log.Println("[LED] Initializing LED service...")
	_ = led.GetService() // Initialize singleton at startup
	log.Println("[LED] LED service initialization complete")

	led.RegisterControllerHeartbeatHandler(func(identifier string, payload *models.LEDControllerHeartbeat) error {
		service := services.NewLEDControllerService()
		_, err := service.RecordHeartbeat(identifier, payload)
		return err
	})

	controllerListener := led.NewControllerListener()
	if controllerListener != nil {
		controllerListener.Start()
	}

	// Ensure auto-admin assignment based on ENV (ADMIN_NAME_MATCH)
	go func() {
		// Run asynchronously to not block startup
		r := services.NewRBACService()
		if err := r.EnsureAutoAdminFromEnv(); err != nil {
			log.Printf("[RBAC] Auto-admin assignment failed: %v", err)
		}
	}()

	// Setup router
	router := mux.NewRouter()

	// API v1 routes
	api := router.PathPrefix("/api/v1").Subrouter()

	// Auth endpoints (public - no auth required)
	api.HandleFunc("/auth/login", handlers.Login).Methods("POST")
	api.HandleFunc("/auth/logout", handlers.Logout).Methods("POST")

	// Health check (public)
	api.HandleFunc("/health", commonhealth.Handler(repository.GetSQLDB(), "warehousecore", "5.9.55")).Methods("GET")

	// Public product pictures (must be accessible without headers for IMG tags)
	api.HandleFunc("/public/products/{id}/pictures/{filename}", handlers.DownloadProductPicture).Methods("GET", "HEAD")
	api.HandleFunc("/public/packages/{id}/pictures/{filename}", handlers.DownloadProductPackagePicture).Methods("GET", "HEAD")

	// Public website feeds (no authentication required)
	api.HandleFunc("/public/products", handlers.GetWebsiteProducts).Methods("GET")
	api.HandleFunc("/public/packages", handlers.GetWebsitePackages).Methods("GET")

	// Protected routes - apply auth middleware
	protected := api.PathPrefix("").Subrouter()
	protected.Use(middleware.AuthMiddleware)

	// Auth status endpoint (requires authentication)
	protected.HandleFunc("/auth/me", handlers.GetCurrentUser).Methods("GET")
	protected.HandleFunc("/auth/change-password", handlers.ChangePassword).Methods("POST")

	// Scan endpoints (CRITICAL - core functionality)
	protected.HandleFunc("/scans", handlers.HandleScan).Methods("POST")
	protected.HandleFunc("/scans/history", handlers.GetScanHistory).Methods("GET")

	// Device endpoints
	api.HandleFunc("/devices", handlers.GetDevices).Methods("GET")
	api.HandleFunc("/devices/tree", handlers.GetDeviceTree).Methods("GET")
	api.HandleFunc("/devices/{id}", handlers.GetDevice).Methods("GET")
	api.HandleFunc("/devices/{id}/status", handlers.UpdateDeviceStatus).Methods("PUT")
	api.HandleFunc("/devices/{id}/movements", handlers.GetDeviceMovements).Methods("GET")

	// Zone endpoints
	api.HandleFunc("/zones", handlers.GetZones).Methods("GET")
	api.HandleFunc("/zones", handlers.CreateZone).Methods("POST")
	api.HandleFunc("/zones/scan", handlers.GetZoneByBarcode).Methods("GET")             // Zone barcode lookup
	api.HandleFunc("/zones/{id}/devices", handlers.GetZoneDevices).Methods("GET")       // Must be before /zones/{id}
	api.HandleFunc("/zones/{id}/devices", handlers.AssignDevicesToZone).Methods("POST") // Assign devices to zone
	api.HandleFunc("/zones/{id}/products", handlers.GetZoneProducts).Methods("GET")     // Get products in zone
	api.HandleFunc("/zones/{id}", handlers.GetZone).Methods("GET")
	api.HandleFunc("/zones/{id}", handlers.UpdateZone).Methods("PUT")
	api.HandleFunc("/zones/{id}", handlers.DeleteZone).Methods("DELETE")
	api.HandleFunc("/zone-types", handlers.GetZoneTypes).Methods("GET")

	// Job endpoints
	api.HandleFunc("/jobs", handlers.GetJobs).Methods("GET")
	api.HandleFunc("/jobs/scan", handlers.GetJobByScan).Methods("GET")
	api.HandleFunc("/jobs/{id}", handlers.GetJobSummary).Methods("GET")
	api.HandleFunc("/jobs/{id}/requirements", handlers.GetJobRequirements).Methods("GET")
	api.HandleFunc("/jobs/{id}/picklist", handlers.GetJobPicklist).Methods("GET")
	api.HandleFunc("/jobs/{id}/picklist/scan", handlers.ScanDeviceToPicklist).Methods("POST")
	api.HandleFunc("/jobs/{id}/complete", handlers.CompleteJob).Methods("POST")

	// Public rental equipment endpoint (for RentalCore integration)
	api.HandleFunc("/rental-equipment", handlers.GetRentalEquipment).Methods("GET")
	api.HandleFunc("/rental-equipment/{id}", handlers.GetRentalEquipmentByID).Methods("GET")
	api.HandleFunc("/rental-field-definitions", handlers.GetRentalFieldDefinitions).Methods("GET")

	// Inventory endpoints
	api.HandleFunc("/inventory/low-stock", handlers.GetLowStockAlerts).Methods("GET")

	// Case endpoints
	api.HandleFunc("/cases", handlers.GetCases).Methods("GET")
	api.HandleFunc("/cases", handlers.CreateCase).Methods("POST")
	api.HandleFunc("/cases/{id}", handlers.GetCase).Methods("GET")
	api.HandleFunc("/cases/{id}", handlers.UpdateCase).Methods("PUT")
	api.HandleFunc("/cases/{id}", handlers.DeleteCase).Methods("DELETE")
	api.HandleFunc("/cases/{id}/contents", handlers.GetCaseContents).Methods("GET")
	api.HandleFunc("/cases/{id}/devices", handlers.AddDevicesToCase).Methods("POST")
	api.HandleFunc("/cases/{id}/devices/{device_id}", handlers.RemoveDeviceFromCase).Methods("DELETE")

	// Maintenance endpoints
	api.HandleFunc("/defects", handlers.GetDefects).Methods("GET")
	api.HandleFunc("/defects", handlers.CreateDefect).Methods("POST")
	api.HandleFunc("/defects/{id}", handlers.UpdateDefect).Methods("PUT")
	api.HandleFunc("/maintenance/inspections", handlers.GetInspections).Methods("GET")
	api.HandleFunc("/maintenance/stats", handlers.GetMaintenanceStats).Methods("GET")

	// Dashboard/stats
	api.HandleFunc("/dashboard/stats", handlers.GetDashboardStats).Methods("GET")
	api.HandleFunc("/movements", handlers.GetMovements).Methods("GET")

	// LED control endpoints
	api.HandleFunc("/led/status", handlers.GetLEDStatus).Methods("GET")
	api.HandleFunc("/led/highlight", handlers.HighlightJobBins).Methods("POST")
	api.HandleFunc("/led/clear", handlers.ClearLEDs).Methods("POST")
	api.HandleFunc("/led/identify", handlers.IdentifyLEDs).Methods("POST")
	api.HandleFunc("/led/test", handlers.TestBin).Methods("POST")
	api.HandleFunc("/led/locate", handlers.LocateBin).Methods("POST")

	// Label generation endpoints
	protected.HandleFunc("/labels/qrcode", handlers.GenerateQRCode).Methods("POST")
	protected.HandleFunc("/labels/barcode", handlers.GenerateBarcode).Methods("POST")
	protected.HandleFunc("/labels/templates", handlers.GetLabelTemplates).Methods("GET")
	protected.HandleFunc("/labels/templates", handlers.CreateLabelTemplate).Methods("POST")
	protected.HandleFunc("/labels/templates/{id}", handlers.GetLabelTemplate).Methods("GET")
	protected.HandleFunc("/labels/templates/{id}", handlers.UpdateLabelTemplate).Methods("PUT")
	protected.HandleFunc("/labels/templates/{id}", handlers.DeleteLabelTemplate).Methods("DELETE")
	protected.HandleFunc("/labels/device/{device_id}", handlers.GenerateDeviceLabel).Methods("POST")
	protected.HandleFunc("/labels/case/{case_id}", handlers.GenerateCaseLabel).Methods("POST")
	protected.HandleFunc("/labels/save", handlers.SaveDeviceLabel).Methods("POST")
	protected.HandleFunc("/labels/save-case", handlers.SaveCaseLabel).Methods("POST")
	protected.HandleFunc("/labels/targets", handlers.ListLabelTargets).Methods("GET")
	protected.HandleFunc("/labels/targets/{target_type}/{target_id}", handlers.GetLabelTarget).Methods("GET")
	protected.HandleFunc("/labels/fields/{target_type}", handlers.GetLabelFields).Methods("GET")
	protected.HandleFunc("/labels/render", handlers.RenderTargetLabel).Methods("POST")
	protected.HandleFunc("/labels/render-batch", handlers.RenderTargetLabels).Methods("POST")
	protected.HandleFunc("/labels/pdf", handlers.ExportLabelsPDF).Methods("POST")
	protected.HandleFunc("/labels/printers", handlers.ListLabelPrinters).Methods("GET")
	protected.HandleFunc("/labels/printers", handlers.CreateLabelPrinter).Methods("POST")
	protected.HandleFunc("/labels/printers/{id}", handlers.UpdateLabelPrinter).Methods("PUT")
	protected.HandleFunc("/labels/printers/{id}", handlers.DeleteLabelPrinter).Methods("DELETE")
	protected.HandleFunc("/labels/print", handlers.PrintLabels).Methods("POST")
	protected.HandleFunc("/labels/print-jobs", handlers.ListLabelPrintJobs).Methods("GET")

	// Admin routes (RBAC protected)
	// Read-only admin routes (admin or manager)
	adminRead := api.PathPrefix("/admin").Subrouter()
	adminRead.Use(middleware.AuthMiddleware)
	adminRead.Use(middleware.RequireAdminOrManager)
	adminRead.HandleFunc("/zone-types", handlers.GetZoneTypes).Methods("GET")
	adminRead.HandleFunc("/zone-types/{id}", handlers.GetZoneType).Methods("GET")
	adminRead.HandleFunc("/led/single-bin-default", handlers.GetLEDSingleBinDefault).Methods("GET")
	adminRead.HandleFunc("/led/job-highlights", handlers.GetLEDJobHighlightSettings).Methods("GET")
	adminRead.HandleFunc("/led/mapping", handlers.GetLEDMapping).Methods("GET")
	adminRead.HandleFunc("/led/controllers", handlers.ListLEDControllers).Methods("GET")
	adminRead.HandleFunc("/led/controllers", handlers.ListLEDControllers).Methods("GET")
	adminRead.HandleFunc("/roles", handlers.GetRoles).Methods("GET")
	adminRead.HandleFunc("/users", handlers.GetUsersWithRoles).Methods("GET")
	adminRead.HandleFunc("/users/{id}/roles", handlers.GetUserRoles).Methods("GET")
	adminRead.HandleFunc("/categories", handlers.GetCategories).Methods("GET")
	adminRead.HandleFunc("/subcategories", handlers.GetSubcategories).Methods("GET")
	adminRead.HandleFunc("/subbiercategories", handlers.GetSubbiercategories).Methods("GET")
	adminRead.HandleFunc("/count-types", handlers.ListCountTypes).Methods("GET")
	adminRead.HandleFunc("/brands", handlers.GetBrands).Methods("GET")
	adminRead.HandleFunc("/manufacturers", handlers.GetManufacturers).Methods("GET")
	adminRead.HandleFunc("/products", handlers.GetProducts).Methods("GET")
	adminRead.HandleFunc("/products/{id}", handlers.GetProduct).Methods("GET")
	adminRead.HandleFunc("/products/{id}/pictures", handlers.GetProductPictures).Methods("GET")
	adminRead.HandleFunc("/products/{id}/pictures/{filename}", handlers.DownloadProductPicture).Methods("GET", "HEAD")
	adminRead.HandleFunc("/product-packages", handlers.GetProductPackages).Methods("GET")
	adminRead.HandleFunc("/product-packages/{id}", handlers.GetProductPackage).Methods("GET")
	adminRead.HandleFunc("/product-packages/{id}/pictures", handlers.GetProductPackagePictures).Methods("GET")
	adminRead.HandleFunc("/product-packages/{id}/pictures/{filename}", handlers.DownloadProductPackagePicture).Methods("GET", "HEAD")
	adminRead.HandleFunc("/rental-equipment", handlers.GetRentalEquipment).Methods("GET")
	adminRead.HandleFunc("/rental-equipment/suppliers", handlers.GetRentalEquipmentSuppliers).Methods("GET")
	adminRead.HandleFunc("/rental-equipment/supplier-contacts", handlers.SearchSupplierContacts).Methods("GET")
	adminRead.HandleFunc("/rental-equipment/{id}", handlers.GetRentalEquipmentByID).Methods("GET")
	adminRead.HandleFunc("/rental-field-definitions", handlers.GetRentalFieldDefinitions).Methods("GET")
	adminRead.HandleFunc("/api-keys", handlers.ListAPIKeys).Methods("GET")
	adminRead.HandleFunc("/cables", handlers.GetAllCables).Methods("GET")
	adminRead.HandleFunc("/cables/{id}", handlers.GetCable).Methods("GET")
	adminRead.HandleFunc("/cable-connectors", handlers.GetCableConnectors).Methods("GET")
	adminRead.HandleFunc("/cable-types", handlers.GetCableTypes).Methods("GET")

	// Admin-only routes (write operations)
	admin := api.PathPrefix("/admin").Subrouter()
	admin.Use(middleware.AuthMiddleware)
	admin.Use(middleware.RequireAdmin)
	admin.HandleFunc("/zone-types", handlers.CreateZoneType).Methods("POST")
	admin.HandleFunc("/zone-types/{id}", handlers.UpdateZoneType).Methods("PUT")
	admin.HandleFunc("/zone-types/{id}", handlers.DeleteZoneType).Methods("DELETE")
	admin.HandleFunc("/led/single-bin-default", handlers.UpdateLEDSingleBinDefault).Methods("PUT")
	admin.HandleFunc("/led/job-highlights", handlers.UpdateLEDJobHighlightSettings).Methods("PUT")
	admin.HandleFunc("/led/mapping", handlers.UpdateLEDMapping).Methods("PUT")
	admin.HandleFunc("/led/mapping/validate", handlers.ValidateLEDMapping).Methods("POST")
	admin.HandleFunc("/led/preview", handlers.PreviewLEDSettings).Methods("POST")
	admin.HandleFunc("/led/controllers/{id}", handlers.UpdateLEDController).Methods("PUT")
	admin.HandleFunc("/led/controllers", handlers.CreateLEDController).Methods("POST")
	admin.HandleFunc("/led/controllers/{id}", handlers.UpdateLEDController).Methods("PUT")
	admin.HandleFunc("/led/controllers/{id}/configure", handlers.ConfigureLEDController).Methods("POST")
	admin.HandleFunc("/led/controllers/{id}/restart", handlers.RestartLEDController).Methods("POST")
	admin.HandleFunc("/led/controllers/{id}", handlers.DeleteLEDController).Methods("DELETE")
	admin.HandleFunc("/users/{id}/roles", handlers.UpdateUserRoles).Methods("PUT")
	admin.HandleFunc("/categories", handlers.CreateCategory).Methods("POST")
	admin.HandleFunc("/categories/{id}", handlers.UpdateCategory).Methods("PUT")
	admin.HandleFunc("/categories/{id}", handlers.DeleteCategory).Methods("DELETE")
	admin.HandleFunc("/subcategories", handlers.CreateSubcategory).Methods("POST")
	admin.HandleFunc("/subcategories/{id}", handlers.UpdateSubcategory).Methods("PUT")
	admin.HandleFunc("/subcategories/{id}", handlers.DeleteSubcategory).Methods("DELETE")
	admin.HandleFunc("/subbiercategories", handlers.CreateSubbiercategory).Methods("POST")
	admin.HandleFunc("/subbiercategories/{id}", handlers.UpdateSubbiercategory).Methods("PUT")
	admin.HandleFunc("/subbiercategories/{id}", handlers.DeleteSubbiercategory).Methods("DELETE")
	admin.HandleFunc("/count-types", handlers.CreateCountType).Methods("POST")
	admin.HandleFunc("/count-types/{id}", handlers.UpdateCountType).Methods("PUT")
	admin.HandleFunc("/count-types/{id}", handlers.DeleteCountType).Methods("DELETE")
	admin.HandleFunc("/brands", handlers.CreateBrand).Methods("POST")
	admin.HandleFunc("/brands/{id}", handlers.UpdateBrand).Methods("PUT")
	admin.HandleFunc("/brands/{id}", handlers.DeleteBrand).Methods("DELETE")
	admin.HandleFunc("/manufacturers", handlers.CreateManufacturer).Methods("POST")
	admin.HandleFunc("/manufacturers/{id}", handlers.UpdateManufacturer).Methods("PUT")
	admin.HandleFunc("/manufacturers/{id}", handlers.DeleteManufacturer).Methods("DELETE")
	admin.HandleFunc("/products", handlers.CreateProduct).Methods("POST")
	admin.HandleFunc("/products/{id}", handlers.UpdateProduct).Methods("PUT")
	admin.HandleFunc("/products/{id}", handlers.DeleteProduct).Methods("DELETE")
	admin.HandleFunc("/products/{id}/restore", handlers.RestoreProduct).Methods("PUT")
	admin.HandleFunc("/products/{id}/website", handlers.UpdateProductWebsite).Methods("PUT")
	admin.HandleFunc("/products/{id}/pictures", handlers.UploadProductPictures).Methods("POST")
	admin.HandleFunc("/products/{id}/pictures/{filename}", handlers.DeleteProductPicture).Methods("DELETE")
	admin.HandleFunc("/products/{id}/devices", handlers.GetProductDevices).Methods("GET")
	admin.HandleFunc("/products/{id}/devices", handlers.CreateDevicesForProduct).Methods("POST")
	admin.HandleFunc("/products/{id}/dependencies", handlers.GetProductDependencies).Methods("GET")
	admin.HandleFunc("/products/{id}/dependencies", handlers.CreateProductDependency).Methods("POST")
	admin.HandleFunc("/products/{id}/dependencies/{dep_id}", handlers.DeleteProductDependency).Methods("DELETE")
	admin.HandleFunc("/product-packages", handlers.CreateProductPackage).Methods("POST")
	admin.HandleFunc("/product-packages/{id}", handlers.UpdateProductPackage).Methods("PUT")
	admin.HandleFunc("/product-packages/{id}", handlers.DeleteProductPackage).Methods("DELETE")
	admin.HandleFunc("/product-packages/{id}/items", handlers.AddItemToPackage).Methods("POST")
	admin.HandleFunc("/product-packages/{package_id}/items/{item_id}", handlers.RemoveItemFromPackage).Methods("DELETE")
	admin.HandleFunc("/product-packages/{id}/pictures", handlers.UploadProductPackagePictures).Methods("POST")
	admin.HandleFunc("/product-packages/{id}/pictures/{filename}", handlers.DeleteProductPackagePicture).Methods("DELETE")
	admin.HandleFunc("/product-packages/{id}/website", handlers.UpdateProductPackageWebsite).Methods("PUT")
	admin.HandleFunc("/rental-equipment", handlers.CreateRentalEquipment).Methods("POST")
	admin.HandleFunc("/rental-equipment/{id}", handlers.UpdateRentalEquipment).Methods("PUT")
	admin.HandleFunc("/rental-equipment/{id}", handlers.DeleteRentalEquipment).Methods("DELETE")
	admin.HandleFunc("/rental-field-definitions", handlers.CreateRentalFieldDefinition).Methods("POST")
	admin.HandleFunc("/rental-field-definitions/{id}", handlers.UpdateRentalFieldDefinition).Methods("PUT")
	admin.HandleFunc("/rental-field-definitions/{id}", handlers.DeleteRentalFieldDefinition).Methods("DELETE")
	admin.HandleFunc("/api-keys", handlers.CreateAPIKey).Methods("POST")
	admin.HandleFunc("/api-keys/{id}/status", handlers.UpdateAPIKeyStatus).Methods("PUT")
	admin.HandleFunc("/api-keys/{id}", handlers.DeleteAPIKey).Methods("DELETE")

	// Cable admin endpoints
	admin.HandleFunc("/cables", handlers.CreateCable).Methods("POST")
	admin.HandleFunc("/cables/{id}", handlers.UpdateCable).Methods("PUT")
	admin.HandleFunc("/cables/{id}", handlers.DeleteCable).Methods("DELETE")
	admin.HandleFunc("/cables/{id}/stock", handlers.SetCableStock).Methods("PUT")
	admin.HandleFunc("/cables/{id}/units", handlers.CreateCableUnits).Methods("POST")
	admin.HandleFunc("/cables/{id}/units/{device_id}", handlers.DeleteCableUnit).Methods("DELETE")
	admin.HandleFunc("/cable-connectors", handlers.CreateCableConnector).Methods("POST")
	admin.HandleFunc("/cable-connectors/{id}", handlers.UpdateCableConnector).Methods("PUT")
	admin.HandleFunc("/cable-connectors/{id}", handlers.DeleteCableConnector).Methods("DELETE")
	admin.HandleFunc("/cable-types", handlers.CreateCableType).Methods("POST")
	admin.HandleFunc("/cable-types/{id}", handlers.UpdateCableType).Methods("PUT")
	admin.HandleFunc("/cable-types/{id}", handlers.DeleteCableType).Methods("DELETE")
	admin.HandleFunc("/devices-list", handlers.GetAllDevicesAdmin).Methods("GET")
	admin.HandleFunc("/devices", handlers.CreateDevice).Methods("POST")
	admin.HandleFunc("/devices/{id}", handlers.GetDeviceAdmin).Methods("GET")
	admin.HandleFunc("/devices/{id}", handlers.UpdateDevice).Methods("PUT")
	admin.HandleFunc("/devices/{id}", handlers.DeleteDevice).Methods("DELETE")
	admin.HandleFunc("/devices/{id}/qr", handlers.GenerateDeviceQR).Methods("GET")
	admin.HandleFunc("/devices/{id}/barcode", handlers.GenerateDeviceBarcode).Methods("GET")
	admin.HandleFunc("/api-limits", handlers.GetAPILimits).Methods("GET")
	admin.HandleFunc("/api-limits", handlers.UpdateAPILimits).Methods("PUT")

	// CSV Export endpoints (read-only, admin or manager)
	adminRead.HandleFunc("/export/{type}", handlers.ExportCSV).Methods("GET")

	// Profile endpoints (authenticated users)
	protected.HandleFunc("/profile/me", handlers.GetMyProfile).Methods("GET")
	protected.HandleFunc("/product-packages/alias-map", handlers.GetProductPackageAliasMap).Methods("GET")

	// Apply middleware
	api.Use(middleware.Logger)
	api.Use(middleware.RecoveryMiddleware)
	api.Use(metrics.Middleware)

	// Serve central branding logos with a fallback to bundled app logos.
	router.PathPrefix("/logos/").HandlerFunc(brandingLogoHandler)

	// Public branding config endpoint (for live polling by SPA)
	router.HandleFunc("/api/v1/branding", func(w http.ResponseWriter, r *http.Request) {
		if brandingSvc == nil {
			http.Error(w, "{}", http.StatusServiceUnavailable)
			return
		}
		cfg := brandingSvc.GetConfig()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cfg)
	}).Methods("GET")

	// Prometheus metrics endpoint
	router.Handle("/metrics", promhttp.Handler())

	// Serve static frontend files with SPA fallback
	router.PathPrefix("/").HandlerFunc(spaHandler)

	// CORS setup
	c := cors.New(cors.Options{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	handler := c.Handler(router)

	// Server configuration
	server := &http.Server{
		Addr:         cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:      handler,
		ReadTimeout:  90 * time.Second,
		WriteTimeout: 90 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("WarehouseCore server starting on %s:%s", cfg.Server.Host, cfg.Server.Port)
		log.Printf("Environment: %s", cfg.App.Environment)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	if controllerListener != nil {
		controllerListener.Close()
	}
	repository.CloseDatabase()
	log.Println("Server stopped")
}
