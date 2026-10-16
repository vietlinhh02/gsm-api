export const commonErrors = [
  ["401", "401", "Thiếu token, token hết hạn hoặc sai scope.", "Lấy token đúng môi trường."],
  ["500", "500", "Không đọc hoặc ghi được dữ liệu.", "Kiểm tra log và kết nối PostgreSQL."],
];

export const endpointDocumentation = {
  services: {
    usage: [
      "Gọi trước khi lấy báo giá để chọn service.id và service_type phù hợp.",
      "Gửi tọa độ trong pickups; thêm service_types để lọc RIDE-TRIP hoặc RIDE-HOUR.",
      "Danh mục hiện là dữ liệu mẫu Hà Nội, chưa kiểm tra vùng phục vụ theo tọa độ.",
    ],
    responseFields: [
      ["data[].service.id", "Dùng làm service_ids[].id khi lấy báo giá."],
      ["data[].service.travel_mode", "Car là ô tô; Motorcycle là xe máy."],
      ["data[].service.capacity", "Số khách tối đa theo dịch vụ mẫu."],
      ["data[].addons", "Dịch vụ bổ sung; gửi id và apply khi lấy báo giá."],
    ],
    errors: [
      ["400", "400", "Thiếu pickups hoặc tọa độ không hợp lệ.", "Gửi ít nhất một điểm đón."],
    ],
    next: ["trip", "hour"],
  },
  trip: {
    usage: [
      "Gửi nhiều service_ids trong cùng request để so sánh ô tô và xe máy trên một hành trình.",
      "Đọc fee_unavailable của từng dịch vụ, kể cả khi HTTP status là 200.",
      "Báo giá có hiệu lực 10 phút. Lấy báo giá mới nếu đổi điểm, thanh toán hoặc addons.",
      "OSRM dùng tuyến driving cho cả ô tô và xe máy; thời gian xe máy chỉ là ước tính.",
    ],
    responseFields: [
      ["data.order_time", "Timestamp đã chuẩn hóa. Chuyển sang số khi gửi create-order."],
      [
        "service_estimates[].fee_unavailable / err_msg",
        "Cho biết dịch vụ có báo giá hay lý do lỗi.",
      ],
      ["estimate_info.fee_id / expires_at", "Mã báo giá và thời điểm hết hạn, theo giây Unix."],
      ["estimate_info.estimate.total_pay", "Số tiền dự kiến thanh toán, đơn vị VND."],
      ["estimate_info.trip", "distance_estimate tính bằng km; duration_estimate tính bằng giây."],
      [
        "estimate_info.trip.geometry",
        "GeoJSON LineString; tọa độ theo thứ tự [longitude, latitude].",
      ],
      ["estimate_info.pricing", "Nguồn bảng giá Hà Nội, các khoản cước và cách làm tròn."],
    ],
    errors: [
      [
        "400",
        "400",
        "Sai points, payment_method hoặc order_time.",
        "Kiểm tra bảng tham số bên dưới.",
      ],
      ["502", "502", "OSRM không tính được tuyến đường.", "Kiểm tra điểm chọn và kết nối OSRM."],
    ],
    next: ["create"],
  },
  hour: {
    usage: [
      "Dùng service_id của RIDE-HOUR; data phải chứa đúng một mục và hour phải lớn hơn 0.",
      "Chỉ cần điểm đón; điền sẵn address và short_address trong báo giá để giữ nguyên khi đặt.",
      "Giá hiện tính 175.000 VND mỗi giờ, trừ discount và không thấp hơn 0 VND.",
      "Giữ order_time đã gửi trong request khi đặt chuyến, kể cả giá trị 0.",
    ],
    responseFields: [
      [
        "service_estimates[].estimate_infos[]",
        "Danh sách gói báo giá; khác estimate_info của RIDE-TRIP.",
      ],
      ["estimate_infos[].fee_id / expires_at", "Báo giá có hiệu lực 10 phút."],
      ["estimate_infos[].service_package_hour", "Số giờ của gói đã chọn."],
      ["estimate_infos[].service_package_distance", "Quãng đường gói mẫu: 15 km mỗi giờ."],
      ["estimate_infos[].total_pay", "Tổng tiền sau discount, đơn vị VND."],
    ],
    errors: [
      ["400", "400", "Sai số mục data, hour hoặc service_id.", "Chọn dịch vụ RIDE-HOUR hợp lệ."],
    ],
    next: ["create"],
  },
  create: {
    usage: [
      "Lấy fee_id từ báo giá cùng Client ID trước khi tạo chuyến.",
      "Giữ nguyên service, order_time, payment_method, tọa độ, tên khách và điện thoại đã báo giá.",
      "address và short_address cũng được đối chiếu. Nên điền đầy đủ ngay khi lấy báo giá.",
      "Lưu data.id để tra cứu, hủy chuyến hoặc đối chiếu webhook. Mỗi lần gọi tạo một chuyến mới.",
    ],
    responseFields: [
      ["data.id", "Order ID của chuyến vừa tạo."],
      ["data.status", "Trạng thái ban đầu FINDING; chưa có tài xế được gán tự động."],
      ["data.path[].id", "Point ID để cập nhật điểm dừng trong Sandbox."],
      ["data.path[].status", "Trạng thái ban đầu CREATED của mỗi điểm."],
      ["data.order_time", "Timestamp Unix dạng chuỗi."],
    ],
    errors: [
      ["400", "400", "fee_id sai, hết hạn hoặc khác môi trường.", "Lấy lại báo giá trước khi đặt."],
      ["400", "400", "Dữ liệu đặt chuyến khác báo giá.", "Giữ nguyên các trường đã lấy báo giá."],
    ],
    next: ["details", "status"],
  },
  details: {
    usage: [
      "Thay {id} trong URL bằng data.id nhận được sau khi đặt chuyến.",
      "Thêm extra_components=driver,path để đọc tài xế, phương tiện và các điểm hành trình.",
      "Nếu bỏ extra_components, response không có path, driver và vehicle.",
      "Chỉ đọc được chuyến của Client ID và môi trường đang xác thực.",
    ],
    responseFields: [
      ["data.status", "Trạng thái hiện tại của chuyến."],
      ["data.driver / vehicle", "Có khi yêu cầu driver; dữ liệu trống trước khi gán tài xế."],
      ["data.path[]", "Các điểm, Point ID và trạng thái; có khi yêu cầu path."],
      ["data.fare", "Báo giá đã chốt khi tạo chuyến; không tính lại theo bảng giá mới."],
      ["data.current_server_time", "Thời gian máy chủ hiện tại, Unix giây dạng chuỗi."],
    ],
    errors: [
      [
        "404",
        "4031",
        "Không tìm thấy chuyến trong phạm vi token.",
        "Kiểm tra Order ID và môi trường.",
      ],
    ],
    next: ["cancel", "webhooks"],
  },
  list: {
    usage: [
      "Gọi GET không có request body để lấy các chuyến của ứng dụng đang xác thực.",
      "Response sắp theo thời điểm tạo, chuyến mới nhất đứng đầu.",
      "Danh sách hiện chưa có pagination hoặc filter. Dùng order_id để gọi trang chi tiết.",
    ],
    responseFields: [
      ["data[]", "Danh sách chuyến; mảng rỗng nếu chưa tạo chuyến nào."],
      [
        "data[].order_id",
        "Order ID để tra cứu chi tiết; tên trường khác data.id của create-order.",
      ],
      ["data[].status / payment_method", "Trạng thái và cách thanh toán của chuyến."],
      ["data[].path", "Các điểm hành trình đã lưu."],
    ],
    errors: [],
    next: ["details"],
  },
  cancel: {
    usage: [
      "Tra cứu trạng thái trước khi hủy; IN PROCESS, COMPLETED và CANCELLED không thể hủy.",
      "cancel_code phải là số dương; 9000 là giá trị trong ví dụ, không phải mặc định máy chủ.",
      "Gửi cancel_message có nội dung để lưu lý do hủy. Hủy thành công phát webhook trạng thái.",
    ],
    responseFields: [
      ["data.message", "ok nghĩa là thao tác đã được ghi nhận."],
      [
        "Chi tiết chuyến: status / cancel_code",
        "Gọi endpoint chi tiết để đọc CANCELLED và lý do hủy.",
      ],
    ],
    errors: [
      ["400", "4017", "Thiếu Order ID hoặc lý do hủy.", "Gửi đủ ba trường trong request."],
      ["404", "4031", "Không có chuyến trong phạm vi token.", "Kiểm tra Order ID và môi trường."],
      [
        "409",
        "4013",
        "Trạng thái chuyến không cho phép hủy.",
        "Đọc trạng thái mới nhất của chuyến.",
      ],
    ],
    next: ["details"],
  },
  geocode: {
    usage: [
      "Forward geocode dùng text để tìm tọa độ; nên thêm tên quận hoặc Hà Nội để thu hẹp kết quả.",
      "Reverse geocode dùng reverse=true cùng lat và lng; không cần text.",
      "max nhận số nguyên từ 1 đến 40. Accept-Language mặc định là vi.",
      "Kết quả được cache 30 ngày. Nominatim công khai có Photon fallback nếu tra cứu thất bại.",
    ],
    responseFields: [
      ["data[].label", "Địa chỉ để hiển thị trong danh sách chọn."],
      ["data[].latitude / longitude", "Tọa độ dùng cho điểm đón hoặc điểm đến."],
      ["data[].place_id", "Mã địa điểm từ provider; không phải Order ID."],
      ["data[].language", "Ngôn ngữ của kết quả tra cứu."],
    ],
    errors: [
      ["400", "400", "Thiếu text, sai tọa độ hoặc max.", "Chọn đúng forward hoặc reverse geocode."],
      ["404", "404", "Không có kết quả phù hợp.", "Đổi từ khóa hoặc chọn trực tiếp trên map."],
      ["502", "502", "Provider không phản hồi.", "Thử lại sau hoặc kiểm tra cấu hình provider."],
    ],
    next: ["services", "trip"],
  },
  status: {
    usage: [
      "Chỉ dùng trong Sandbox, với token scope sandbox.ride.trips và chuyến cùng môi trường.",
      "Luồng thường dùng: FINDING → ASSIGNED → IN PROCESS → COMPLETED.",
      "ASSIGNED thêm tài xế và phương tiện mẫu. Backend không tự chạy chuỗi trạng thái này.",
      "update_type=point cần point_id trong path; cập nhật điểm không đổi status của chuyến.",
      "status nhận WAITING_FOR_PAYMENT, SCHEDULING, FINDING, ASSIGNED, IN PROCESS, COMPLETED, CANCELLED.",
      "point_status nhận CREATED, ARRIVING, NEARED, NEARED_50M, ARRIVED, COMPLETED hoặc FAILED.",
    ],
    responseFields: [
      ["data.message", "ok khi cập nhật thành công; đọc lại chuyến để xem dữ liệu mới."],
    ],
    errors: [
      ["400", "400", "Sai update_type hoặc enum trạng thái.", "Gửi đúng chuỗi, kể cả dấu cách."],
      ["404", "4031 / 4041", "Không có Order ID hoặc Point ID.", "Lấy ID từ chuyến trong Sandbox."],
      [
        "409",
        "4013",
        "Chuyến đã kết thúc hoặc không thể hủy.",
        "Tạo chuyến mới để tiếp tục mô phỏng.",
      ],
    ],
    next: ["details", "webhooks"],
  },
};

export const guideDocumentation = {
  auth: {
    sections: [
      {
        id: "token-response",
        title: "Đọc token response",
        paragraphs: [
          "Response gồm access_token, token_type, expires_in và scope, không bọc trong data. " +
            "expires_in tính bằng giây; hiện là 2.592.000 giây (30 ngày).",
          "token_type là Bearer. Ride request chỉ cần access token, không gửi kèm Client secret.",
        ],
        rows: [["Sandbox", "/v1/sandbox/token · scope sandbox.ride.trips"]],
      },
      {
        id: "auth-errors",
        title: "Khi xác thực thất bại",
        paragraphs: [
          "Chưa có refresh token. Token hết hạn thì gọi lại token endpoint để lấy token mới.",
        ],
        rows: [
          ["400 · Token endpoint", "Kiểm tra grant_type, scope và Content-Type form-urlencoded."],
          ["401 · Token endpoint", "Client ID hoặc Client secret không đúng."],
          ["401 · Ride endpoint", "Token thiếu, hết hạn hoặc có scope khác môi trường."],
        ],
      },
    ],
  },
  webhooks: {
    sections: [
      {
        id: "delivery",
        title: "Nhận và xác nhận event",
        paragraphs: [
          "WEBHOOK_URL nhận POST application/json khi tạo, hủy hoặc đổi trạng thái. " +
            "Receiver phải trả HTTP 200; status khác, kể cả 204, được xem là thất bại.",
          "Thử tối đa 7 lần, thời gian chờ tăng từ 30 giây lên 16 phút. " +
            "Lưu event_id để xử lý idempotency và trả 200 cho event đã xử lý.",
        ],
        rows: [
          ["order_status_changed", "data có order_id và status của chuyến."],
          ["point_status_changed", "data có order_id, point_id và point_status của điểm."],
          ["event_time", "Thời điểm tạo event, Unix giây dạng số."],
          ["resource_href", "URL chi tiết chuyến trong Sandbox."],
        ],
      },
      {
        id: "signature",
        title: "Kiểm tra chữ ký",
        paragraphs: [
          "WEBHOOK_SIGNING_KEY bật X-VSF-Signature. Tính HMAC-SHA256 trên raw body với cùng key, " +
            "đổi sang hex chữ thường để đối chiếu; không parse rồi stringify lại JSON.",
          "Khi đổi key, WEBHOOK_SIGNING_KEY_SECONDARY gửi thêm X-VSF-Signature-Secondary. " +
            "Bỏ trống signing key thì header chữ ký tương ứng không được gửi.",
        ],
      },
      {
        id: "receiver-auth",
        title: "Authorization của receiver",
        paragraphs: [
          "Dùng WEBHOOK_AUTHORIZATION để đặt Authorization trực tiếp, hoặc WEBHOOK_TOKEN_URL " +
            "để lấy Bearer token bằng WEBHOOK_CLIENT_ID và WEBHOOK_CLIENT_SECRET.",
          "WEBHOOK_SCOPE là tùy chọn; token được dùng lại tới hết expires_in. " +
            "X-Environment có giá trị sandbox.",
        ],
      },
    ],
  },
  "green-connect": {
    sections: [
      {
        id: "booking-flow",
        title: "Chuẩn bị và đặt chuyến",
        paragraphs: [
          "Lấy báo giá RIDE-TRIP với partner_pay và points theo thứ tự đón rồi trả. " +
            "create-order dùng fee_id còn hiệu lực cùng service_id, service_type, order_time và points.",
          "Thêm partner_payment_method=B2B và partner_trip_id. " +
            "Giữ nguyên tọa độ, tên, điện thoại và địa chỉ đã báo giá.",
        ],
        rows: [
          [
            "PICK UP",
            "Đúng một điểm; address_note bắt đầu bằng Đón khách và có dấu phân cách ' - '.",
          ],
          [
            "DROP OFF",
            "Ít nhất một điểm; address_note bắt đầu bằng Trả khách và có dấu phân cách ' - '.",
          ],
          ["partner_trip_id", "Tham chiếu tối đa 32 byte; không thay thế data.id của chuyến."],
        ],
      },
      {
        id: "follow-up",
        title: "Theo dõi sau khi đặt",
        paragraphs: [
          "Lưu Order ID để tra cứu với extra_components=driver,path hoặc nhận trạng thái qua webhook. " +
            "Đối chiếu Order ID với partner_trip_id bạn đã lưu.",
          "Ví dụ bên dưới gồm thông tin Green Connect; thêm các trường bắt buộc từ báo giá.",
        ],
      },
    ],
  },
};
