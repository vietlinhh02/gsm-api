"use client";

import { useCallback, useMemo, useSyncExternalStore } from "react";
import { restoreInbox } from "#lib/webhook-inbox.mjs";

function subscribe(listener) {
  window.addEventListener("storage", listener);
  window.addEventListener("gsm-webhook-inbox", listener);
  return () => {
    window.removeEventListener("storage", listener);
    window.removeEventListener("gsm-webhook-inbox", listener);
  };
}

const serverSnapshot = () => null;

export function useSavedInbox(key) {
  const snapshot = useCallback(() => {
    try {
      return sessionStorage.getItem(key);
    } catch {
      return "storage-unavailable";
    }
  }, [key]);
  const value = useSyncExternalStore(subscribe, snapshot, serverSnapshot);
  return useMemo(() => {
    if (!value) return { inbox: null, error: "" };
    try {
      return { inbox: restoreInbox(value, window.location.origin), error: "" };
    } catch {
      return {
        inbox: null,
        error: "Không khôi phục được hộp thư đã lưu. Bạn có thể tạo link mới.",
      };
    }
  }, [value]);
}
