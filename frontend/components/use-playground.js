"use client";

import { useState } from "react";
import { requestApi, apiErrorMessage } from "#lib/http.mjs";
import {
  initialState,
  changeEnvironment,
  chooseEndpoint,
  findEndpoint,
  parseRequestBody,
  recordResult,
  validatePath,
} from "#lib/playground.mjs";

function failureMessage(error) {
  if (error.name === "TimeoutError") return "Yêu cầu quá thời gian chờ. Hãy thử lại.";
  if (error instanceof TypeError) return "Kết nối bị gián đoạn. Kiểm tra mạng rồi thử lại.";
  return error.message ?? "Không gửi được request. Kiểm tra kết nối API.";
}

export function usePlayground(endpointId) {
  const [state, setState] = useState(() => initialState(endpointId));
  const endpoint = findEndpoint(state.endpointId);
  const patch = (values) => setState((previous) => ({ ...previous, ...values }));
  const selectEnvironment = (value) => setState((previous) => changeEnvironment(previous, value));
  const selectEndpoint = (id) => setState((previous) => chooseEndpoint(previous, id));

  async function authenticate() {
    patch({ busy: true, error: "", result: null, token: "" });
    try {
      const sandbox = state.environment === "sandbox";
      const body = new URLSearchParams({
        client_id: state.clientId,
        client_secret: state.clientSecret,
        grant_type: "client_credentials",
        scope: sandbox ? "sandbox.ride.trips" : "ride.trips",
      });
      const result = await requestApi({
        path: sandbox ? "/v1/sandbox/token" : "/v1/public/token",
        method: "POST",
        body,
        contentType: "application/x-www-form-urlencoded",
      });
      const accessToken = tokenFromResponse(result);
      patch({
        token: accessToken,
        result: {
          ...result,
          text: JSON.stringify(
            { ...result.data, access_token: "(đã lưu trong trường Bearer token)" },
            null,
            2,
          ),
        },
        busy: false,
      });
    } catch (error) {
      patch({ error: failureMessage(error), busy: false });
    }
  }

  async function send() {
    patch({ busy: true, error: "", result: null });
    try {
      validatePath(state.path);
      const request = parseRequestBody(endpoint.method, state.body);
      const result = await requestApi({
        path: state.path,
        method: endpoint.method,
        token: state.token,
        body: endpoint.method === "POST" ? JSON.stringify(request) : undefined,
      });
      setState((previous) =>
        recordResult(
          {
            ...previous,
            drafts: { ...previous.drafts, [endpoint.id]: state.body },
          },
          result,
          request,
        ),
      );
    } catch (error) {
      patch({ error: failureMessage(error), busy: false });
    }
  }

  return { state, endpoint, patch, selectEnvironment, selectEndpoint, authenticate, send };
}

function tokenFromResponse(result) {
  if (result.status !== 200 || !result.data?.access_token) {
    throw new Error(apiErrorMessage(result));
  }
  return result.data.access_token;
}
