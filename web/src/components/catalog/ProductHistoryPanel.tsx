import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "../../lib/api";
import { catalogError } from "../../lib/catalog";
import { WorkspaceLoading, WorkspaceNotice } from "./Workspace";
interface HistoryEvent {
  id: number;
  action: string;
  timestamp: string;
  user_id: number | null;
}
export function ProductHistoryPanel({ id }: { id: number }) {
  const { t } = useTranslation();
  const [events, setEvents] = useState<HistoryEvent[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const result = await api.get<HistoryEvent[]>(
        `/admin/products/${id}/history`,
      );
      setEvents(result.data);
    } catch (err) {
      setError(catalogError(err, t("catalog.loadFailed")));
    } finally {
      setLoading(false);
    }
  }, [id, t]);
  useEffect(() => {
    void load();
  }, [load]);
  if (loading) return <WorkspaceLoading />;
  if (error)
    return (
      <WorkspaceNotice error retry={() => void load()}>
        {error}
      </WorkspaceNotice>
    );
  return (
    <div className="suite-table-wrap">
      <table>
        <thead>
          <tr>
            <th>{t("catalog.date")}</th>
            <th>{t("catalog.change")}</th>
            <th>{t("catalog.actor")}</th>
          </tr>
        </thead>
        <tbody>
          {events.map((event) => (
            <tr key={event.id}>
              <td>{new Date(event.timestamp).toLocaleString()}</td>
              <td>
                {t(`catalog.historyActions.${event.action}`, {
                  defaultValue: t("catalog.productChanged"),
                })}
              </td>
              <td>{event.user_id ?? "—"}</td>
            </tr>
          ))}
          {!events.length && (
            <tr>
              <td colSpan={3}>{t("catalog.noHistory")}</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
