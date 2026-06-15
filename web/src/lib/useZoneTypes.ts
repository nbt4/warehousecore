import { useCallback, useEffect, useState } from 'react';
import { zoneTypesApi, type ZoneTypeDefinition } from './api';
import { toast } from './toast';

export function useZoneTypes() {
  const [zoneTypes, setZoneTypes] = useState<ZoneTypeDefinition[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadZoneTypes = useCallback(async () => {
    setLoading(true);
    setError(null);

    try {
      const { data } = await zoneTypesApi.getAll();
      setZoneTypes(data);
    } catch (err) {
      toast.error('Failed to load zone types:' + " " + String(err));
      setError('Lagertypen konnten nicht geladen werden.');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadZoneTypes();
  }, [loadZoneTypes]);

  return { zoneTypes, loading, error, reload: loadZoneTypes };
}
