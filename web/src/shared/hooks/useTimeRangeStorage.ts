import { useSessionStorage } from "./useSessionStorage";

interface TimeRangeStorage {
  value: string;
  start?: string;
  end?: string;
}

const TIME_RANGE_STORAGE_KEY = "timeRange";

export function useTimeRangeStorage() {
  const [stored, setStored] = useSessionStorage<TimeRangeStorage | null>(
    TIME_RANGE_STORAGE_KEY,
    null,
  );

  const saveTimeRange = (
    value: string,
    start?: string | null,
    end?: string | null,
  ) => {
    setStored({
      value,
      start: start ?? undefined,
      end: end ?? undefined,
    });
  };

  const getStoredTimeRange = (): TimeRangeStorage | null => {
    return stored;
  };

  return { stored, saveTimeRange, getStoredTimeRange };
}
