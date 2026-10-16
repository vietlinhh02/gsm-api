"use client";

import { useEffect, useRef, useState } from "react";

const locationErrors = {
  1: "Bạn đã từ chối quyền vị trí. Bật quyền vị trí trong trình duyệt rồi thử lại.",
  2: "Thiết bị chưa xác định được vị trí. Kiểm tra GPS hoặc chọn điểm trên bản đồ.",
  3: "Lấy vị trí quá thời gian chờ. Thử lại hoặc chọn điểm trên bản đồ.",
};

export function useCurrentLocation(onLocation) {
  const [state, setState] = useState({ busy: false, error: "", accuracy: null });
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  function locate() {
    if (!window.isSecureContext) {
      setState({
        busy: false,
        error: "Mở trang bằng HTTPS hoặc localhost để dùng vị trí.",
        accuracy: null,
      });
      return;
    }
    if (!navigator.geolocation) {
      setState({ busy: false, error: "Trình duyệt này không hỗ trợ lấy vị trí.", accuracy: null });
      return;
    }
    setState({ busy: true, error: "", accuracy: null });
    navigator.geolocation.getCurrentPosition(
      ({ coords }) => {
        if (!mounted.current) return;
        const { latitude, longitude, accuracy } = coords;
        onLocation({
          latitude,
          longitude,
          label: `Vị trí của tôi · ${latitude.toFixed(6)}, ${longitude.toFixed(6)}`,
        });
        setState({ busy: false, error: "", accuracy: Math.round(accuracy) });
      },
      (error) => {
        if (!mounted.current) return;
        setState({
          busy: false,
          error: locationErrors[error.code] ?? "Không lấy được vị trí.",
          accuracy: null,
        });
      },
      { enableHighAccuracy: true, timeout: 15000, maximumAge: 30000 },
    );
  }

  return { ...state, locate };
}
