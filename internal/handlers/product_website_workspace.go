package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type websiteContent struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Visible     bool     `json:"website_visible"`
	Featured    bool     `json:"website_featured"`
	Images      []string `json:"website_images"`
	Thumbnail   *string  `json:"website_thumbnail"`
}

type websiteWorkspace struct {
	Draft             websiteContent             `json:"draft"`
	Published         *websiteContent            `json:"published"`
	Revision          int64                      `json:"revision"`
	PublishedRevision int64                      `json:"published_revision"`
	State             string                     `json:"state"`
	Sync              services.PublicationStatus `json:"sync"`
}

func loadWebsiteWorkspace(tx *sql.Tx, kind string, id int, lock bool) (websiteWorkspace, string, error) {
	var name, status string
	var description, rawImages sql.NullString
	var current websiteContent
	query := `SELECT name,description,lifecycle_status,website_visible,website_featured,website_thumbnail,website_images_json FROM products WHERE productid=$1`
	if kind == "package" {
		query = `SELECT name,description,CASE WHEN is_active THEN 'active' ELSE 'archived' END,website_visible,FALSE,website_image_url,website_images_json FROM product_packages WHERE id=$1`
	}
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(query, id).Scan(&name, &description, &status, &current.Visible, &current.Featured, &current.Thumbnail, &rawImages)
	if err != nil {
		return websiteWorkspace{}, status, err
	}
	current.Title = name
	current.Description = description.String
	current.Images = parseStringSlice(rawImages)
	state := websiteWorkspace{Draft: current, State: "draft"}
	if current.Visible {
		copy := current
		state.Published = &copy
		state.State = "published"
	}
	var draft, published []byte
	err = tx.QueryRow(`SELECT draft,published,revision,published_revision FROM warehouse_website_content WHERE entity_type=$1 AND entity_id=$2`, kind, id).
		Scan(&draft, &published, &state.Revision, &state.PublishedRevision)
	if err != nil && err != sql.ErrNoRows {
		return state, status, err
	}
	if len(draft) > 0 {
		if err = json.Unmarshal(draft, &state.Draft); err != nil {
			return state, status, err
		}
	}
	if len(published) > 0 {
		var c websiteContent
		if err = json.Unmarshal(published, &c); err != nil {
			return state, status, err
		}
		if current.Visible {
			state.Published = &c
		}
	}
	if state.Revision > state.PublishedRevision && current.Visible {
		state.State = "changes_pending"
	}

	return state, status, nil
}

func getWebsiteSettings(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid ID"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht geladen werden"})
		return
	}
	defer tx.Rollback()
	state, _, err := loadWebsiteWorkspace(tx, kind, id, false)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Artikel nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht geladen werden"})
		return
	}
	tx.Rollback()
	state.Sync, _ = websiteRevalidator.Status("/products")
	respondJSON(w, 200, state)
}
func GetProductWebsite(w http.ResponseWriter, r *http.Request) { getWebsiteSettings(w, r, "product") }
func GetProductPackageWebsite(w http.ResponseWriter, r *http.Request) {
	getWebsiteSettings(w, r, "package")
}

func saveWebsiteSettings(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	if err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid ID"})
		return
	}
	var input struct {
		Action           string          `json:"action"`
		Title            *string         `json:"title"`
		Description      *string         `json:"description"`
		Visible          *bool           `json:"website_visible"`
		Featured         *bool           `json:"website_featured"`
		Images           []string        `json:"website_images"`
		Thumbnail        json.RawMessage `json:"website_thumbnail"`
		ExpectedRevision *int64          `json:"expected_revision"`
	}
	if err = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		respondJSON(w, 400, map[string]string{"error": "Invalid website settings"})
		return
	}
	if input.Action == "" {
		input.Action = "publish"
		if input.Visible != nil && !*input.Visible {
			input.Action = "unpublish"
		}
	}
	if input.Action != "publish" && input.Action != "save_draft" && input.Action != "unpublish" {
		respondJSON(w, 400, map[string]string{"error": "Invalid publication action"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht gespeichert werden"})
		return
	}
	defer tx.Rollback()
	state, lifecycle, err := loadWebsiteWorkspace(tx, kind, id, true)
	if err == sql.ErrNoRows {
		respondJSON(w, 404, map[string]string{"error": "Artikel nicht gefunden"})
		return
	}
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht gespeichert werden"})
		return
	}
	if input.ExpectedRevision != nil && *input.ExpectedRevision != state.Revision {
		respondJSON(w, 409, map[string]string{"error": "Website-Inhalte wurden inzwischen geändert. Bitte neu laden."})
		return
	}
	draft := state.Draft
	if input.Title != nil {
		draft.Title = strings.TrimSpace(*input.Title)
	}
	if input.Description != nil {
		draft.Description = *input.Description
	}
	if input.Visible != nil {
		draft.Visible = *input.Visible
	}
	if input.Featured != nil {
		draft.Featured = *input.Featured
	}
	if input.Images != nil {
		draft.Images = sanitizeWebsiteImages(input.Images)
	}
	if len(input.Thumbnail) > 0 {
		draft.Thumbnail = nil
		if string(input.Thumbnail) != "null" {
			var thumbnail string
			if json.Unmarshal(input.Thumbnail, &thumbnail) != nil {
				respondJSON(w, 400, map[string]string{"error": "Invalid cover image"})
				return
			}
			if strings.TrimSpace(thumbnail) != "" {
				draft.Thumbnail = &thumbnail
			}
		}
	}
	if len(draft.Title) > 255 || len(draft.Description) > 50000 {
		respondJSON(w, 400, map[string]string{"error": "Website-Text ist zu lang"})
		return
	}
	if input.Action == "publish" {
		if lifecycle != "active" {
			respondJSON(w, 409, map[string]string{"error": "Archivierte Artikel können nicht veröffentlicht werden"})
			return
		}
		if draft.Title == "" {
			respondJSON(w, 400, map[string]string{"error": "Bitte einen öffentlichen Titel angeben"})
			return
		}
		if input.ExpectedRevision != nil && len(draft.Images) == 0 {
			respondJSON(w, 400, map[string]string{"error": "Bitte mindestens ein Website-Bild auswählen"})
			return
		}
		if err := validateWebsitePictures(tx, kind, id, draft.Images, draft.Thumbnail); err != nil {
			respondJSON(w, 503, map[string]string{"error": err.Error()})
			return
		}
		draft.Visible = true
	} else if input.Action == "unpublish" {
		draft.Visible = false
		draft.Featured = false
	}
	if draft.Thumbnail != nil {
		selected := false
		for _, image := range draft.Images {
			if image == *draft.Thumbnail {
				selected = true
			}
		}
		if !selected {
			draft.Thumbnail = nil
		}
	}
	if draft.Thumbnail == nil && len(draft.Images) > 0 {
		draft.Thumbnail = &draft.Images[0]
	}
	if kind == "package" {
		draft.Featured = false
	}
	encoded, _ := json.Marshal(draft)
	publish := input.Action != "save_draft"
	_, err = tx.Exec(`INSERT INTO warehouse_website_content(entity_type,entity_id,draft,published,revision,published_revision,published_at)
 VALUES($1,$2,$3,CASE WHEN $4 THEN $3::jsonb ELSE NULL END,1,CASE WHEN $4 THEN 1 ELSE 0 END,CASE WHEN $4 THEN now() ELSE NULL END)
 ON CONFLICT(entity_type,entity_id) DO UPDATE SET draft=$3,revision=warehouse_website_content.revision+1,updated_at=now(),
 published=CASE WHEN $4 THEN $3::jsonb ELSE warehouse_website_content.published END,
 published_revision=CASE WHEN $4 THEN warehouse_website_content.revision+1 ELSE warehouse_website_content.published_revision END,
 published_at=CASE WHEN $4 THEN now() ELSE warehouse_website_content.published_at END`, kind, id, string(encoded), publish)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht gespeichert werden"})
		return
	}
	state.Draft = draft
	state.Revision++
	state.State = "draft"
	if publish {
		if kind == "product" {
			_, err = tx.Exec(`UPDATE products SET website_visible=$1,website_featured=$2,website_thumbnail=$3,website_images_json=$4,updated_at=now() WHERE productid=$5`, draft.Visible, draft.Featured, draft.Thumbnail, marshalStringSlice(draft.Images), id)
		} else {
			_, err = tx.Exec(`UPDATE product_packages SET website_visible=$1,website_image_url=$2,website_images_json=$3,updated_at=now() WHERE id=$4`, draft.Visible, draft.Thumbnail, marshalStringSlice(draft.Images), id)
		}
		if err == nil {
			state.Sync, err = queueWebsiteUpdate(tx)
		}
		state.PublishedRevision = state.Revision
		if draft.Visible {
			copy := draft
			state.Published = &copy
			state.State = "published"
		} else {
			state.Published = nil
		}
	} else if state.Published != nil {
		state.State = "changes_pending"
	}
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht gespeichert werden"})
		return
	}
	if kind == "product" {
		err = recordProductAudit(tx, r, "product.website."+input.Action, id, nil, draft)
	} else {
		err = recordCatalogAudit(tx, r, "package", strconv.Itoa(id), "package.website."+input.Action, draft)
	}
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website-Änderung konnte nicht protokolliert werden"})
		return
	}

	if err = tx.Commit(); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website konnte nicht gespeichert werden"})
		return
	}
	respondJSON(w, 200, map[string]any{"draft": state.Draft, "published": state.Published, "revision": state.Revision, "published_revision": state.PublishedRevision, "state": state.State, "sync": state.Sync,
		"website_visible": draft.Visible, "website_featured": draft.Featured, "website_images": draft.Images, "website_thumbnail": draft.Thumbnail})
}

func queueWebsiteUpdate(tx *sql.Tx) (services.PublicationStatus, error) {
	result := services.PublicationStatus{Path: "/products", Status: "disabled"}
	if websiteRevalidator.Configured() {
		result.Status = "pending"
	}
	err := tx.QueryRow(`INSERT INTO website_revalidation(path,generation,status,requested_at,next_attempt) VALUES('/products',1,$1,now(),now())
 ON CONFLICT(path) DO UPDATE SET generation=website_revalidation.generation+1,status=EXCLUDED.status,requested_at=now(),next_attempt=now(),attempts=0,last_error=''
 RETURNING generation,delivered_generation`, result.Status).Scan(&result.Generation, &result.DeliveredGeneration)
	return result, err
}

func validateWebsitePictures(tx *sql.Tx, kind string, id int, images []string, thumbnail *string) error {
	known := map[string]bool{}
	rows, err := tx.Query(`SELECT file_name FROM warehouse_picture_files WHERE entity_type=$1 AND entity_id=$2`, kind, id)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		known[name] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	missing := false
	var deleting int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM warehouse_picture_deletions WHERE entity_type=$1 AND entity_id=$2 AND file_name=ANY($3)`, kind, id, pq.Array(images)).Scan(&deleting); err != nil {
		return err
	}
	if deleting > 0 {
		return errors.New("Ein ausgewähltes Bild wird gelöscht")
	}

	for _, image := range images {
		if !known[image] {
			missing = true
		}
	}
	if thumbnail != nil && !known[*thumbnail] {
		missing = true
	}
	if missing {
		if !productPictureService.Enabled() {
			return errors.New("Bilderablage ist nicht verfügbar")
		}
		var folder string
		if err = tx.QueryRow(`SELECT folder_name FROM warehouse_picture_folders WHERE entity_type=$1 AND entity_id=$2`, kind, id).Scan(&folder); err != nil {
			return errors.New("Bilderablage konnte nicht geladen werden")
		}
		pictures, err := productPictureService.ListPictures(folder)
		if err != nil {
			return errors.New("Website-Bilder konnten nicht geprüft werden")
		}
		for _, picture := range pictures {
			known[picture.FileName] = true
		}
	}
	for _, image := range images {
		if !known[image] {
			return errors.New("Ein ausgewähltes Website-Bild ist nicht mehr vorhanden")
		}
	}
	if thumbnail != nil && !known[*thumbnail] {
		return errors.New("Das Titelbild ist nicht mehr vorhanden")
	}
	return nil
}

func GetWebsitePublicationStatus(w http.ResponseWriter, r *http.Request) {
	status, err := websiteRevalidator.Status("/products")
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website-Status konnte nicht geladen werden"})
		return
	}
	respondJSON(w, 200, status)
}
func RetryWebsitePublication(w http.ResponseWriter, r *http.Request) {
	status, err := websiteRevalidator.Queue("/products")
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Website-Aktualisierung konnte nicht gestartet werden"})
		return
	}
	respondJSON(w, 202, status)
}

func recordUploadedPictures(kind string, id int, folder string, pictures []services.ProductPictureInfo) error {
	tx, err := repository.GetSQLDB().Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var active bool
	query := `SELECT lifecycle_status='active' FROM products WHERE productid=$1 FOR UPDATE`
	if kind == "package" {
		query = `SELECT COALESCE(is_active,true) FROM product_packages WHERE id=$1 FOR UPDATE`
	}
	if err = tx.QueryRow(query, id).Scan(&active); err != nil || !active {
		return errors.New("article is no longer active")
	}
	if _, err = tx.Exec(`INSERT INTO warehouse_picture_folders(entity_type,entity_id,folder_name) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, kind, id, folder); err != nil {
		return err
	}
	for _, picture := range pictures {
		if _, err = tx.Exec(`INSERT INTO warehouse_picture_files(entity_type,entity_id,file_name,size,content_type,modified_at) VALUES($1,$2,$3,$4,$5,$6)`, kind, id, picture.FileName, picture.Size, picture.ContentType, picture.ModifiedAt); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

type workspacePicture struct {
	FileName     string    `json:"file_name"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	ModifiedAt   time.Time `json:"modified_at"`
	DownloadURL  string    `json:"download_url"`
	ThumbnailURL string    `json:"thumbnail_url"`
	PreviewURL   string    `json:"preview_url"`
}

func pictureResponses(kind string, id int, pictures []services.ProductPictureInfo) []workspacePicture {
	route := "products"
	if kind == "package" {
		route = "product-packages"
	}
	result := make([]workspacePicture, 0, len(pictures))
	for _, picture := range pictures {
		base := fmt.Sprintf("/api/v1/admin/%s/%d/pictures/%s", route, id, url.PathEscape(picture.FileName))
		result = append(result, workspacePicture{picture.FileName, picture.Size, picture.ContentType, picture.ModifiedAt, base, base + "?variant=thumb", base + "?variant=preview"})
	}
	return result
}

// Serialize media deletion with publication, then remove every live/draft reference.
func deleteWorkspacePicture(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := strconv.Atoi(mux.Vars(r)["id"])
	filename, decodeErr := url.PathUnescape(mux.Vars(r)["filename"])
	if err != nil || decodeErr != nil || filename == "" {
		respondJSON(w, 400, map[string]string{"error": "Invalid picture ID"})
		return
	}
	tx, err := repository.GetSQLDB().BeginTx(r.Context(), nil)
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bild konnte nicht entfernt werden"})
		return
	}
	defer tx.Rollback()
	state, _, err := loadWebsiteWorkspace(tx, kind, id, true)
	if err != nil {
		respondJSON(w, 404, map[string]string{"error": "Artikel nicht gefunden"})
		return
	}
	var folder string
	if err = tx.QueryRow(`SELECT folder_name FROM warehouse_picture_folders WHERE entity_type=$1 AND entity_id=$2`, kind, id).Scan(&folder); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bilderablage konnte nicht geladen werden"})
		return
	}
	if _, err = tx.Exec(`INSERT INTO warehouse_picture_deletions(entity_type,entity_id,folder_name,file_name) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, kind, id, folder, filename); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bildlöschung konnte nicht vorgemerkt werden"})
		return
	}

	clean := func(content websiteContent) websiteContent {
		remaining := make([]string, 0, len(content.Images))
		for _, image := range content.Images {
			if image != filename {
				remaining = append(remaining, image)
			}
		}
		content.Images = remaining
		if content.Thumbnail != nil && *content.Thumbnail == filename {
			content.Thumbnail = nil
			if len(remaining) > 0 {
				content.Thumbnail = &remaining[0]
			}
		}
		return content
	}
	state.Draft = clean(state.Draft)
	draftJSON, _ := json.Marshal(state.Draft)
	var publishedJSON any
	var live websiteContent
	if state.Published != nil {
		live = clean(*state.Published)
		encoded, _ := json.Marshal(live)
		publishedJSON = string(encoded)
		state.Published = &live
	}
	_, err = tx.Exec(`DELETE FROM warehouse_picture_files WHERE entity_type=$1 AND entity_id=$2 AND file_name=$3`, kind, id, filename)
	if err == nil {
		_, err = tx.Exec(`UPDATE warehouse_website_content SET draft=$3::jsonb,published=$4::jsonb,revision=revision+1,
 published_revision=CASE WHEN revision=published_revision THEN revision+1 ELSE published_revision END,updated_at=now() WHERE entity_type=$1 AND entity_id=$2`, kind, id, string(draftJSON), publishedJSON)
	}
	// Legacy metadata remains the public image contract. Read it separately when
	// no published workspace exists (legacy articles can still be visible).
	if state.Published == nil {
		var raw sql.NullString
		var thumb *string
		query := `SELECT website_images_json,website_thumbnail FROM products WHERE productid=$1`
		if kind == "package" {
			query = `SELECT website_images_json,website_image_url FROM product_packages WHERE id=$1`
		}
		if queryErr := tx.QueryRow(query, id).Scan(&raw, &thumb); queryErr != nil {
			err = queryErr
		} else {
			live = clean(websiteContent{Images: parseStringSlice(raw), Thumbnail: thumb})
		}
	}
	if err == nil {
		if kind == "product" {
			_, err = tx.Exec(`UPDATE products SET website_images_json=$1,website_thumbnail=$2,updated_at=now() WHERE productid=$3`, marshalStringSlice(live.Images), live.Thumbnail, id)
		} else {
			_, err = tx.Exec(`UPDATE product_packages SET website_images_json=$1,website_image_url=$2,updated_at=now() WHERE id=$3`, marshalStringSlice(live.Images), live.Thumbnail, id)
		}
	}
	if err == nil && kind == "product" {
		err = recordProductAudit(tx, r, "product.picture.delete", id, nil, map[string]any{"file_name": filename})
	}
	if err == nil {
		state.Sync, err = queueWebsiteUpdate(tx)
	}
	if err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bildreferenzen konnten nicht aktualisiert werden. Bitte erneut versuchen."})
		return
	}
	// No workspace row exists for legacy articles until their first draft save.
	var revision int64
	queryErr := tx.QueryRow(`SELECT revision FROM warehouse_website_content WHERE entity_type=$1 AND entity_id=$2`, kind, id).Scan(&revision)
	if queryErr != nil && queryErr != sql.ErrNoRows {
		respondJSON(w, 500, map[string]string{"error": "Bildänderung konnte nicht gespeichert werden"})
		return
	}
	if err = tx.Commit(); err != nil {
		respondJSON(w, 500, map[string]string{"error": "Bildänderung konnte nicht gespeichert werden"})
		return
	}
	pending := true
	if err = productPictureService.DeletePicture(folder, filename); err == nil {
		_, err = repository.GetSQLDB().Exec(`DELETE FROM warehouse_picture_deletions WHERE entity_type=$1 AND entity_id=$2 AND file_name=$3`, kind, id, filename)
		pending = err != nil
	}
	respondJSON(w, 200, map[string]any{"revision": revision, "sync": state.Sync, "deletion_pending": pending})
}

func publicCatalogPictureAllowed(r *http.Request, kind string, id int, filename string) bool {
	var deleting bool
	if err := repository.GetSQLDB().QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM warehouse_picture_deletions WHERE entity_type=$1 AND entity_id=$2 AND file_name=$3)`, kind, id, filename).Scan(&deleting); err != nil || deleting {
		return false
	}
	if strings.Contains(r.URL.Path, "/admin/") {
		return true
	}
	var allowed bool
	query := `SELECT lifecycle_status='active' AND COALESCE(website_visible,false) AND (website_thumbnail=$2 OR COALESCE(website_images_json,'[]')::jsonb ? $2) FROM products WHERE productid=$1`
	if kind == "package" {
		query = `SELECT COALESCE(is_active,true) AND COALESCE(website_visible,false) AND (website_image_url=$2 OR COALESCE(website_images_json,'[]')::jsonb ? $2) FROM product_packages WHERE id=$1`
	}
	err := repository.GetSQLDB().QueryRowContext(r.Context(), query, id, filename).Scan(&allowed)
	return err == nil && allowed
}

func filterPendingPictures(kind string, id int, pictures []services.ProductPictureInfo) ([]services.ProductPictureInfo, error) {
	rows, err := repository.GetSQLDB().Query(`SELECT file_name FROM warehouse_picture_deletions WHERE entity_type=$1 AND entity_id=$2`, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pending := map[string]bool{}
	for rows.Next() {
		var file string
		if err = rows.Scan(&file); err != nil {
			return nil, err
		}
		pending[file] = true
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	result := make([]services.ProductPictureInfo, 0, len(pictures))
	for _, picture := range pictures {
		if !pending[picture.FileName] {
			result = append(result, picture)
		}
	}
	return result, nil
}
func resumePictureDeletions() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if !productPictureService.Enabled() {
			continue
		}
		rows, err := repository.GetSQLDB().Query(`SELECT entity_type,entity_id,folder_name,file_name FROM warehouse_picture_deletions ORDER BY created_at LIMIT 20`)
		if err != nil {
			continue
		}
		type deletion struct {
			kind         string
			id           int
			folder, file string
		}
		tasks := []deletion{}
		for rows.Next() {
			var task deletion
			if rows.Scan(&task.kind, &task.id, &task.folder, &task.file) == nil {
				tasks = append(tasks, task)
			}
		}
		rows.Close()
		for _, task := range tasks {
			if productPictureService.DeletePicture(task.folder, task.file) == nil {
				_, _ = repository.GetSQLDB().Exec(`DELETE FROM warehouse_picture_deletions WHERE entity_type=$1 AND entity_id=$2 AND file_name=$3`, task.kind, task.id, task.file)
			}
		}
	}
}
