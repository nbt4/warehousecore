import { useCatalogWriteAccess, useUnsavedChanges } from "./useCatalog";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
} from "react";
import { useTranslation } from "react-i18next";
import {
  ArrowLeft,
  ArrowRight,
  ExternalLink,
  Upload,
  Trash2,
  Eye,
  Save,
} from "lucide-react";
import { prepareProductImage } from "../../lib/prepare-product-images";
import { api, type ProductPicture } from "../../lib/api";
import { appPath } from "../../lib/app-paths";
import { catalogError } from "../../lib/catalog";
import { WorkspaceField, WorkspaceLoading, WorkspaceNotice } from "./Workspace";

export interface WebsiteContent {
  title: string;
  description: string;
  website_visible: boolean;
  website_featured: boolean;
  website_images: string[];
  website_thumbnail: string | null;
}
export interface WebsiteSync {
  status: "idle" | "disabled" | "pending" | "processing" | "synced" | "error";
  generation: number;
  delivered_generation: number;
  last_error?: string;
}
export interface WebsiteWorkspace {
  draft: WebsiteContent;
  published: WebsiteContent | null;
  revision: number;
  published_revision: number;
  state: "draft" | "published" | "changes_pending";
  sync: WebsiteSync;
}

export function WebsitePanel({
  id,
  kind = "product",
  onChanged,
  readOnly = false,
}: {
  id: number;
  kind?: "product" | "package";
  onChanged?: () => void;
  readOnly?: boolean;
}) {
  const { t } = useTranslation();
  const permitted = useCatalogWriteAccess();
  const writable = permitted && !readOnly;
  const endpoint = `/admin/${kind === "product" ? "products" : "product-packages"}/${id}`;
  const [workspace, setWorkspace] = useState<WebsiteWorkspace | null>(null);
  const [content, setContent] = useState<WebsiteContent | null>(null);
  const [pictures, setPictures] = useState<ProductPicture[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [optimize, setOptimize] = useState(true);
  const [preparing, setPreparing] = useState(false);
  const [progress, setProgress] = useState(0);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState("");
  const [preview, setPreview] = useState(false);
  const previews = useRef<string[]>([]);
  const generation = useRef(0);
  const load = useCallback(async () => {
    const current = ++generation.current;
    setLoading(true);
    setError("");
    try {
      const [website, images] = await Promise.all([
        api.get<WebsiteWorkspace>(`${endpoint}/website`),
        api.get<{ pictures: ProductPicture[] }>(`${endpoint}/pictures`),
      ]);
      if (generation.current !== current) return;
      setWorkspace(website.data);
      setContent(website.data.draft);
      setPictures(images.data.pictures ?? []);
    } catch (err) {
      if (generation.current === current) setError(catalogError(err));
    } finally {
      if (generation.current === current) setLoading(false);
    }
  }, [endpoint]);
  const cancelLoad = useCallback(() => {
    generation.current++;
  }, []);
  useEffect(() => {
    void load();
    return cancelLoad;
  }, [load, cancelLoad]);
  useEffect(
    () => () => {
      previews.current.forEach(URL.revokeObjectURL);
    },
    [],
  );
  const syncStatus = workspace?.sync.status;
  useEffect(() => {
    if (!["pending", "processing", "error"].includes(syncStatus ?? "")) return;
    const timer = window.setInterval(() => {
      void api
        .get<WebsiteSync>("/admin/website-status")
        .then(({ data }) =>
          setWorkspace((old) => (old ? { ...old, sync: data } : old)),
        )
        .catch(() => {});
    }, 1500);
    return () => window.clearInterval(timer);
  }, [syncStatus]);
  const dirty =
    content !== null &&
    workspace !== null &&
    JSON.stringify(content) !== JSON.stringify(workspace.draft);
  useUnsavedChanges(dirty || uploading, t("catalog.confirmDiscard"), () => {
    generation.current++;
    setPictures((old) => old.filter((picture) => !picture.temporary));
    setSaving(false);
    setContent(workspace?.draft ?? null);
    setUploading(false);
    setPreparing(false);
    previews.current.forEach(URL.revokeObjectURL);
    previews.current = [];
  });
  const update = (changes: Partial<WebsiteContent>) => {
    setContent((old) => (old ? { ...old, ...changes } : old));
    setMessage("");
  };
  const upload = async (event: ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files ?? []);
    event.target.value = "";
    if (!files.length || !content) return;
    if (
      files.length > 20 ||
      files.some((file) => file.size > 10 * 1024 * 1024)
    ) {
      setError(t("catalog.imageLimits"));
      return;
    }
    const current = generation.current;
    setUploading(true);
    setProgress(0);
    setError("");
    const temporary = files.map((file) => {
      const url = URL.createObjectURL(file);
      previews.current.push(url);
      return {
        file_name: url,
        download_url: url,
        thumbnail_url: url,
        size: file.size,
        content_type: file.type,
        modified_at: new Date().toISOString(),
        temporary: true,
      } satisfies ProductPicture;
    });
    setPictures((old) => [...old, ...temporary]);
    const form = new FormData();
    try {
      setPreparing(true);
      for (const file of files) {
        const prepared = optimize ? await prepareProductImage(file) : file;
        if (generation.current !== current) return;
        form.append("files", prepared);
      }
      setPreparing(false);
      const response = await api.post<{
        pictures: ProductPicture[];
        uploaded_files: string[];
      }>(`${endpoint}/pictures`, form, {
        headers: { "Content-Type": "multipart/form-data" },
        timeout: 90000,
        onUploadProgress: (event) => {
          if (generation.current !== current) return;
          setProgress(
            Math.round(
              (event.loaded / (event.total || event.loaded || 1)) * 100,
            ),
          );
        },
      });
      if (generation.current !== current) return;
      setPictures((old) => [
        ...old.filter((picture) => !picture.temporary),
        ...response.data.pictures,
      ]);
      setContent((old) =>
        old
          ? {
              ...old,
              website_images: [
                ...old.website_images,
                ...response.data.uploaded_files,
              ],
              website_thumbnail:
                old.website_thumbnail ||
                response.data.uploaded_files[0] ||
                null,
            }
          : old,
      );
      setMessage(t("catalog.imagesReady"));
    } catch (err) {
      if (generation.current !== current) return;
      setPictures((old) => old.filter((picture) => !picture.temporary));
      setError(catalogError(err, t("catalog.uploadFailed")));
    } finally {
      if (generation.current === current) {
        setPreparing(false);
        setUploading(false);
      }
      temporary.forEach((picture) => URL.revokeObjectURL(picture.download_url));
      previews.current = previews.current.filter(
        (url) => !temporary.some((picture) => picture.download_url === url),
      );
    }
  };
  const save = async (action: "save_draft" | "publish" | "unpublish") => {
    if (!content || !workspace) return;
    const current = generation.current;
    setSaving(true);
    setError("");
    setMessage("");
    try {
      const { data } = await api.put<WebsiteWorkspace>(`${endpoint}/website`, {
        ...content,
        action,
        expected_revision: workspace.revision,
      });
      if (generation.current !== current) return;
      setWorkspace(data);
      setContent(data.draft);
      setMessage(
        t(
          action === "save_draft"
            ? "catalog.draftSaved"
            : action === "publish"
              ? "catalog.publicationQueued"
              : "catalog.unpublished",
        ),
      );
      onChanged?.();
    } catch (err) {
      if (generation.current === current)
        setError(catalogError(err, t("catalog.saveFailed")));
    } finally {
      if (generation.current === current) setSaving(false);
    }
  };
  const remove = async (picture: ProductPicture) => {
    if (!window.confirm(t("catalog.deleteImageConfirm"))) return;
    setError("");
    setSaving(true);
    try {
      const { data } = await api.delete<{ revision: number }>(
        `${endpoint}/pictures/${encodeURIComponent(picture.file_name)}`,
      );
      setWorkspace((old) => (old ? { ...old, revision: data.revision } : old));
      setPictures((old) =>
        old.filter((item) => item.file_name !== picture.file_name),
      );
      const next =
        content?.website_images.filter((name) => name !== picture.file_name) ??
        [];
      update({
        website_images: next,
        website_thumbnail:
          content?.website_thumbnail === picture.file_name
            ? (next[0] ?? null)
            : content?.website_thumbnail,
      });
    } catch (err) {
      setError(catalogError(err, t("catalog.deleteFailed")));
    } finally {
      setSaving(false);
    }
  };
  const move = (name: string, direction: number) => {
    if (!content) return;
    const images = [...content.website_images];
    const index = images.indexOf(name);
    const target = index + direction;
    if (target < 0 || target >= images.length) return;
    [images[index], images[target]] = [images[target], images[index]];
    update({ website_images: images });
  };
  if (loading) return <WorkspaceLoading />;
  if (!content || !workspace)
    return (
      <WorkspaceNotice error retry={() => void load()}>
        {error || t("catalog.loadFailed")}
      </WorkspaceNotice>
    );
  const selected = content.website_images
    .map((name) => pictures.find((picture) => picture.file_name === name))
    .filter((picture): picture is ProductPicture => !!picture);
  const ordered = [
    ...selected,
    ...pictures.filter(
      (picture) => !content.website_images.includes(picture.file_name),
    ),
  ];
  return (
    <div className="space-y-5">
      {error && <WorkspaceNotice error>{error}</WorkspaceNotice>}
      {message && <WorkspaceNotice>{message}</WorkspaceNotice>}
      <section className="suite-card p-5 space-y-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-xl font-bold">{t("catalog.websiteContent")}</h2>
            <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
              {t(`catalog.websiteState.${workspace.state}`)}
              {dirty ? ` · ${t("catalog.unsaved")}` : ""}
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <button
              className="suite-button"
              onClick={() => setPreview(!preview)}
            >
              <Eye size={16} />
              {t("catalog.preview")}
            </button>
            {workspace.published && (
              <a
                className="suite-button"
                target="_blank"
                rel="noreferrer"
                href="https://tsunami-events.de/produkte"
              >
                <ExternalLink size={16} />
                {t("catalog.openWebsite")}
              </a>
            )}
          </div>
        </div>
        <WorkspaceField label={t("catalog.publicTitle")}>
          <input
            value={content.title}
            disabled={!writable || saving}
            onChange={(event) => update({ title: event.target.value })}
            maxLength={255}
          />
        </WorkspaceField>
        <WorkspaceField
          label={t("catalog.publicDescription")}
          hint={t("catalog.publicDescriptionHelp")}
        >
          <textarea
            rows={5}
            value={content.description}
            disabled={!writable || saving}
            onChange={(event) => update({ description: event.target.value })}
          />
        </WorkspaceField>
        {kind === "product" && (
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={content.website_featured}
              disabled={!writable || saving}
              onChange={(event) =>
                update({ website_featured: event.target.checked })
              }
            />
            {t("catalog.featured")}
          </label>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p
            className="text-sm"
            role="status"
            style={{
              color:
                workspace.sync.status === "error"
                  ? "var(--color-error)"
                  : "var(--text-secondary)",
            }}
          >
            {t(`catalog.sync.${workspace.sync.status}`)}
          </p>
          {writable &&
            ["error", "disabled"].includes(workspace.sync.status) && (
              <button
                className="suite-button"
                onClick={() => {
                  void api
                    .post<WebsiteSync>("/admin/website-status/retry")
                    .then(({ data }) =>
                      setWorkspace((old) =>
                        old ? { ...old, sync: data } : old,
                      ),
                    )
                    .catch((err) =>
                      setError(catalogError(err, t("catalog.saveFailed"))),
                    );
                }}
              >
                {t("catalog.retry")}
              </button>
            )}
        </div>
      </section>
      <section className="suite-card p-5 space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-xl font-bold">{t("catalog.images")}</h2>
            <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
              {t("catalog.imagesHelp")}
            </p>
            {writable && (
              <label className="flex gap-2 items-center text-sm">
                <input
                  type="checkbox"
                  checked={optimize}
                  disabled={uploading}
                  onChange={(event) => setOptimize(event.target.checked)}
                />
                {t("catalog.optimizeImages")}
              </label>
            )}
          </div>
          {writable && (
            <label className="suite-button cursor-pointer">
              <Upload size={16} />
              {t("catalog.upload")}
              <input
                type="file"
                accept="image/jpeg,image/png,image/gif,image/webp,image/heic"
                multiple
                className="sr-only"
                disabled={uploading || saving}
                onChange={(event) => void upload(event)}
              />
            </label>
          )}
        </div>
        {uploading && (
          <div role="status">
            <p className="text-sm">
              {t(
                preparing
                  ? "catalog.imagePreparing"
                  : progress < 100
                    ? "catalog.uploadProgress"
                    : "catalog.imageProcessing",
                { progress },
              )}
            </p>
            <progress className="w-full" max={100} value={progress} />
          </div>
        )}
        {!ordered.length && (
          <p style={{ color: "var(--text-secondary)" }}>
            {t("catalog.noImages")}
          </p>
        )}
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {ordered.map((picture) => {
            const chosen = content.website_images.includes(picture.file_name);
            const position = content.website_images.indexOf(picture.file_name);
            return (
              <div key={picture.file_name} className="space-y-3 min-w-0">
                <img
                  loading="lazy"
                  src={
                    picture.temporary
                      ? picture.download_url
                      : appPath(picture.thumbnail_url || picture.download_url)
                  }
                  alt={`${content.title} – ${t("catalog.image")}`}
                  className="h-40 w-full rounded-lg object-contain"
                  style={{ background: "var(--surface-2)" }}
                />
                {picture.temporary ? (
                  <p role="status">{t("catalog.imageProcessing")}</p>
                ) : (
                  <>
                    <label className="flex items-center gap-2 text-sm">
                      <input
                        type="checkbox"
                        checked={chosen}
                        disabled={!writable || saving || uploading}
                        onChange={() => {
                          const images = chosen
                            ? content.website_images.filter(
                                (name) => name !== picture.file_name,
                              )
                            : [...content.website_images, picture.file_name];
                          update({
                            website_images: images,
                            website_thumbnail:
                              content.website_thumbnail === picture.file_name &&
                              chosen
                                ? (images[0] ?? null)
                                : content.website_thumbnail ||
                                  picture.file_name,
                          });
                        }}
                      />
                      {t("catalog.useOnWebsite")}
                    </label>
                    <div className="flex flex-wrap items-center gap-2">
                      <label className="flex items-center gap-2 text-sm">
                        <input
                          type="radio"
                          name={`cover-${kind}-${id}`}
                          checked={
                            content.website_thumbnail === picture.file_name
                          }
                          disabled={!writable || !chosen || saving || uploading}
                          onChange={() =>
                            update({ website_thumbnail: picture.file_name })
                          }
                        />
                        {t("catalog.cover")}
                      </label>
                      {writable && (
                        <>
                          <button
                            className="suite-button"
                            disabled={
                              !chosen || position === 0 || saving || uploading
                            }
                            onClick={() => move(picture.file_name, -1)}
                            aria-label={t("catalog.imageEarlier")}
                          >
                            <ArrowLeft size={16} />
                          </button>
                          <button
                            className="suite-button"
                            disabled={
                              !chosen ||
                              position === content.website_images.length - 1 ||
                              saving ||
                              uploading
                            }
                            onClick={() => move(picture.file_name, 1)}
                            aria-label={t("catalog.imageLater")}
                          >
                            <ArrowRight size={16} />
                          </button>
                          <button
                            className="suite-button"
                            disabled={saving || uploading}
                            onClick={() => void remove(picture)}
                            aria-label={t("catalog.deleteImage")}
                          >
                            <Trash2 size={16} />
                          </button>
                        </>
                      )}
                    </div>
                  </>
                )}
              </div>
            );
          })}
        </div>
      </section>
      {preview && (
        <section
          className="suite-card p-5 space-y-4"
          aria-label={t("catalog.preview")}
        >
          <h2 className="text-xl font-bold">{content.title}</h2>
          {selected[0] && (
            <img
              src={appPath(
                selected.find(
                  (image) => image.file_name === content.website_thumbnail,
                )?.preview_url ||
                  selected[0].preview_url ||
                  selected[0].download_url,
              )}
              alt={content.title}
              className="max-h-80 w-full object-contain"
            />
          )}
          <p className="whitespace-pre-wrap">{content.description}</p>
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
            {t("catalog.previewHelp")}
          </p>
        </section>
      )}
      {writable && (
        <div className="flex flex-wrap justify-end gap-2">
          <button
            className="suite-button"
            disabled={saving || uploading}
            onClick={() => void save("save_draft")}
          >
            <Save size={16} />
            {t("catalog.saveDraft")}
          </button>
          {workspace.published && (
            <button
              className="suite-button"
              disabled={saving || uploading}
              onClick={() => void save("unpublish")}
            >
              {t("catalog.unpublish")}
            </button>
          )}
          <button
            className="suite-button suite-button--primary"
            disabled={
              saving ||
              uploading ||
              !content.website_images.length ||
              !content.title.trim()
            }
            onClick={() => void save("publish")}
          >
            {t(saving ? "catalog.saving" : "catalog.publish")}
          </button>
        </div>
      )}
    </div>
  );
}
