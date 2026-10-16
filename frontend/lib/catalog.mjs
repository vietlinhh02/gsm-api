export const points = [
  {
    type: "PICK UP",
    latitude: 21.0287,
    longitude: 105.8524,
    name: "Nguyễn Văn A",
    phone_number: "+84900000000",
    short_address: "Hồ Hoàn Kiếm",
    address: "Hồ Hoàn Kiếm, Hà Nội",
  },
  {
    type: "DROP OFF",
    latitude: 21.0324,
    longitude: 105.812,
    name: "Nguyễn Văn A",
    phone_number: "+84900000000",
    short_address: "Lotte Center",
    address: "54 Liễu Giai, Hà Nội",
  },
];

const field = (name, type, required, description) => ({
  name,
  type,
  required: required ? "Bắt buộc" : "Tùy chọn",
  description,
});

export const pointFields = [
  field("type", "string", true, 'Dùng chính xác "PICK UP" hoặc "DROP OFF".'),
  field("latitude", "number", true, "Vĩ độ trong khoảng -90 đến 90."),
  field("longitude", "number", true, "Kinh độ trong khoảng -180 đến 180."),
  field("name", "string", true, "Tên khách; tối đa 256 byte UTF-8."),
  field("phone_number", "string", true, "Số điện thoại quốc tế bắt đầu bằng +."),
  field("short_address", "string", false, "Bắt buộc khi đặt chuyến; tối đa 512 byte UTF-8."),
  field("address", "string", false, "Bắt buộc khi đặt chuyến; tối đa 512 byte UTF-8."),
  field("address_note", "string", false, "Ghi chú cho tài xế; tối đa 512 byte UTF-8."),
];

export const endpoints = [
  {
    id: "services",
    title: "Dịch vụ khả dụng",
    english: "Available services",
    method: "POST",
    path: "/v1/ride/list-available-services",
    group: "Khám phá dịch vụ",
    description: "Tìm loại xe phục vụ tại điểm đón trước khi lấy báo giá.",
    fields: [
      field("pickups", "Point[]", true, "Ít nhất một tọa độ latitude, longitude."),
      field("dropoffs", "Point[]", false, "Tọa độ trả khách để lọc thêm dịch vụ."),
      field("service_types", "string[]", false, "RIDE-TRIP hoặc RIDE-HOUR."),
      field("order_time", "int64", false, "Thời gian Unix theo giây (UTC), không âm."),
    ],
    sample: {
      pickups: [{ latitude: points[0].latitude, longitude: points[0].longitude }],
      dropoffs: [{ latitude: points[1].latitude, longitude: points[1].longitude }],
      service_types: ["RIDE-TRIP"],
      order_time: 0,
    },
    response: {
      data: [
        {
          service: {
            id: 2179,
            display_name: "Green Car",
            service_type: "RIDE-TRIP",
            travel_mode: "Car",
            capacity: 4,
          },
          addons: [],
        },
      ],
    },
    notes: ["Danh sách dịch vụ và khu vực phục vụ là dữ liệu mẫu."],
  },
  {
    id: "trip",
    title: "Ước tính chuyến",
    english: "Estimate RIDE-TRIP",
    method: "POST",
    path: "/v1/ride/estimate/order",
    group: "Báo giá",
    description: "Lấy báo giá theo tuyến đường. Dùng fee_id để đặt chuyến theo đúng báo giá này.",
    fields: [
      field("service_ids", "object[]", true, "Danh sách { id, addons? }; không được rỗng."),
      field("order_time", "int64", true, "0/null là hiện tại; tối đa 14 ngày trong tương lai."),
      field("payment_method", "string", true, "cash hoặc partner_pay."),
      field("points", "Point[]", true, "Điểm đón và trả theo thứ tự hành trình."),
    ],
    sample: { service_ids: [{ id: 2179 }], order_time: 0, payment_method: "cash", points },
    response: {
      data: {
        order_time: "1770350401",
        service_estimates: [
          {
            fee_unavailable: false,
            service: { id: 2179, service_type: "RIDE-TRIP" },
            estimate_info: {
              fee_id: "550e8400-e29b-41d4-a716-446655440000",
              expires_at: 1770351001,
              estimate: { total_fee: 47550, total_pay: 47550, currency_code: "VND" },
              trip: { distance_estimate: 3.1, duration_estimate: 600 },
            },
          },
        ],
      },
    },
    notes: [
      "Nếu request báo giá gửi order_time = 0, dùng order_time trả về khi đặt chuyến.",
      "Dịch vụ bổ sung nằm trong service_ids[].addons. Mỗi mục cần id và apply.",
      "OSRM cung cấp tuyến đường; giá cước được tính theo công thức mô phỏng.",
      "Giá tham khảo Hà Nội tháng 3/2025: ô tô tính bậc km, xe máy tính km và phút.",
      "estimate_info.pricing chứa nguồn, từng khoản và cách làm tròn; trip.geometry là GeoJSON.",
    ],
    hasPoints: true,
  },
  {
    id: "hour",
    title: "Ước tính theo giờ",
    english: "Estimate RIDE-HOUR",
    method: "POST",
    path: "/v1/ride/estimate/hour-order",
    group: "Báo giá",
    description: "Lấy giá gói thuê xe theo giờ. Chọn dịch vụ RIDE-HOUR rồi dùng fee_id để đặt.",
    fields: [
      field("data", "object[]", true, "Mỗi lần chỉ gửi một request."),
      field("data[].id", "string", true, "Trường dự phòng; gửi chuỗi rỗng."),
      field("data[].service_id", "int64", true, "ID dịch vụ RIDE-HOUR."),
      field("data[].hour", "int64", true, "Số giờ của gói; lớn hơn 0."),
      field("data[].order_time", "int64", true, "Thời gian Unix theo giây; 0 là hiện tại."),
      field("data[].payment_method", "string", true, "cash hoặc partner_pay."),
      field("data[].promo_code", "string", true, "Chuỗi rỗng khi không dùng."),
      field("data[].discount", "int64", true, "0 khi không giảm giá."),
      field("data[].expired_at", "int64", true, "0 khi không chỉ định."),
      field("data[].distance", "int64", true, "0 khi không chỉ định."),
      field("data[].points", "Point[]", true, "Tọa độ điểm đón và thông tin khách."),
    ],
    sample: {
      data: [
        {
          id: "",
          service_id: 1903,
          hour: 2,
          order_time: 0,
          promo_code: "",
          discount: 0,
          payment_method: "cash",
          expired_at: 0,
          distance: 0,
          points: [points[0]],
        },
      ],
    },
    response: {
      data: {
        service_estimates: [
          {
            fee_unavailable: false,
            service: { id: 1903, service_type: "RIDE-HOUR" },
            estimate_infos: [
              {
                fee_id: "550e8400-e29b-41d4-a716-446655440000",
                service_package_hour: 2,
                service_package_distance: 30,
                total_fee: 350000,
                total_pay: 350000,
              },
            ],
          },
        ],
      },
    },
    notes: ["Khi đặt chuyến, giữ nguyên order_time đã gửi để lấy báo giá theo giờ."],
    hasPoints: true,
  },
  {
    id: "create",
    title: "Đặt chuyến",
    english: "Create a ride",
    method: "POST",
    path: "/v1/ride/create-order",
    group: "Quản lý chuyến",
    description:
      "Đặt chuyến từ báo giá còn hiệu lực. Loại xe, thanh toán, điểm dừng và dịch vụ bổ sung phải khớp báo giá.",
    fields: [
      field("service_id", "integer", true, "Mã dịch vụ đã lấy báo giá."),
      field("service_type", "string", true, "RIDE-TRIP hoặc RIDE-HOUR."),
      field("fee_id", "uuid", true, "Mã báo giá chưa hết hạn."),
      field("order_time", "int64", true, "Thời điểm đã ghi trong báo giá."),
      field("payment_method", "string", true, "cash hoặc partner_pay, khớp báo giá."),
      field("points", "Point[]", true, "Các điểm hành trình, kèm địa chỉ đầy đủ."),
      field("addons", "object[]", false, "Cấp cao nhất: { id, apply: true }; khớp báo giá."),
      field("partner_trip_id", "string", false, "Tham chiếu nội bộ; tối đa 32 byte UTF-8."),
      field("partner_payment_method", "string", false, "B2B đối với Green Connect."),
    ],
    sample: {
      service_id: 2179,
      service_type: "RIDE-TRIP",
      fee_id: "<fee_id>",
      order_time: 0,
      payment_method: "cash",
      points,
    },
    response: {
      data: {
        id: "550e8400-e29b-41d4-a716-446655440000",
        status: "FINDING",
        order_time: "1770350401",
        path: points,
      },
    },
    notes: ["Sau khi có báo giá, bấm Dùng báo giá để đặt trong playground."],
    hasPoints: true,
  },
  {
    id: "details",
    title: "Chi tiết chuyến",
    english: "Ride details",
    method: "GET",
    path: "/v1/ride/orders/{id}?extra_components=driver,path",
    group: "Quản lý chuyến",
    description: "Lấy trạng thái hiện tại, tài xế và các điểm trên chuyến.",
    fields: [
      field("id", "path string", true, "Mã chuyến đi đã tạo."),
      field(
        "extra_components",
        "query string",
        false,
        "driver,path để nhận thêm thông tin tài xế và hành trình.",
      ),
    ],
    response: {
      data: {
        id: "<order_id>",
        status: "FINDING",
        payment_method: "cash",
        order_time: "1770350401",
        path: points,
      },
    },
    notes: ["Các mốc thời gian trả về là thời gian Unix theo giây, ở dạng chuỗi."],
  },
  {
    id: "list",
    title: "Danh sách chuyến",
    english: "List rides",
    method: "GET",
    path: "/v1/ride/list-user-orders",
    group: "Quản lý chuyến",
    description: "Liệt kê chuyến đi của ứng dụng đang xác thực, chuyến mới nhất ở đầu danh sách.",
    fields: [],
    response: {
      data: [
        {
          order_id: "<order_id>",
          status: "FINDING",
          order_time: "1770350401",
          payment_method: "cash",
        },
      ],
    },
    notes: ["Chưa hỗ trợ bộ lọc vì tài liệu API chưa quy định tham số lọc."],
  },
  {
    id: "cancel",
    title: "Hủy chuyến",
    english: "Cancel a ride",
    method: "POST",
    path: "/v1/ride/cancel-order",
    group: "Quản lý chuyến",
    description:
      "Hủy chuyến trước khi đón khách. Không thể hủy chuyến đang đi, đã hoàn thành hoặc đã hủy.",
    fields: [
      field("order_id", "string", true, "ID chuyến cần hủy."),
      field("cancel_code", "integer", true, "Mã lý do hủy lớn hơn 0; ví dụ 9000."),
      field("cancel_message", "string", true, "Lý do hủy chuyến."),
    ],
    sample: { order_id: "<order_id>", cancel_code: 9000, cancel_message: "Khách hủy chuyến" },
    response: { data: { message: "ok" } },
    notes: [
      "Đọc cả HTTP status và code trong JSON: 4013 là sai trạng thái, 4031 là không có chuyến.",
    ],
  },
  {
    id: "geocode",
    title: "Tra cứu địa chỉ",
    english: "Geocode / reverse geocode",
    method: "GET",
    path: "/v1/place/geocode?text=Van%20Mieu%20Ha%20Noi&max=5",
    group: "Bản đồ",
    description: "Tìm tọa độ từ địa chỉ hoặc tìm địa chỉ từ tọa độ.",
    fields: [
      field("text", "query string", false, "Địa chỉ cần tìm tọa độ."),
      field("reverse", "query boolean", false, "true để tìm địa chỉ từ tọa độ."),
      field("lat / lng", "query number", false, "Bắt buộc khi reverse=true."),
      field("max", "query integer", false, "Số kết quả từ 1 đến 40; mặc định 5."),
    ],
    response: {
      data: [
        {
          label: "Văn Miếu, Đống Đa, Hà Nội",
          place_id: "osm-way-123",
          latitude: 21.0278,
          longitude: 105.8355,
          language: "vi",
        },
      ],
    },
    notes: [
      "Tìm địa chỉ từ tọa độ: /v1/place/geocode?reverse=true&lat=21.0287&lng=105.8524&max=5",
      "Nominatim công khai giới hạn một request mỗi giây.",
      "Dùng dịch vụ chuyên dụng khi triển khai chính thức.",
    ],
  },
  {
    id: "status",
    title: "Mô phỏng trạng thái",
    english: "Sandbox status",
    method: "POST",
    path: "/v1/sandbox/ride/update-status",
    group: "Trạng thái chuyến",
    sandboxOnly: true,
    description: "Thay đổi trạng thái chuyến đi hoặc điểm dừng và gửi thông báo qua webhook.",
    fields: [
      field("update_type", "string", true, "order hoặc point."),
      field("order_id", "string", true, "Mã chuyến đi trong Sandbox."),
      field("status", "string", false, "Bắt buộc khi update_type=order."),
      field("point_id", "string", false, "Bắt buộc khi update_type=point."),
      field("point_status", "string", false, "Bắt buộc khi update_type=point."),
    ],
    sample: { update_type: "order", order_id: "<order_id>", status: "ASSIGNED" },
    response: { data: { message: "ok" } },
    notes: [
      "Dùng access token có scope sandbox.ride.trips. Chuyến Production không dùng endpoint này.",
    ],
  },
];

export const navigation = [
  {
    title: "Bắt đầu",
    items: [
      { id: "overview", title: "Tổng quan", href: "/" },
      { id: "auth", title: "Xác thực", href: "/docs/auth" },
    ],
  },
  {
    title: "API endpoints",
    items: endpoints.map(({ id, title, method }) => ({
      id,
      title,
      method,
      href: "/docs/" + id,
    })),
  },
  {
    title: "Tích hợp",
    items: [
      { id: "webhooks", title: "Webhooks", href: "/docs/webhooks" },
      { id: "green-connect", title: "Green Connect", href: "/docs/green-connect" },
      { id: "playground", title: "Playground", href: "/playground" },
    ],
  },
];

export const guides = {
  auth: {
    title: "Xác thực",
    description: "Xác thực ứng dụng bằng OAuth 2.0",
    paragraphs: [
      "Access token xác thực ứng dụng, không đại diện cho khách hàng. " +
        "Gửi client_id, client_secret, grant_type=client_credentials và scope bằng form-urlencoded.",
      "Đính kèm Authorization: Bearer <access_token> khi gọi API. " +
        "Chọn token endpoint và scope theo môi trường trong bảng dưới.",
    ],
    code: [
      "curl -X POST https://gsm-api.103.176.113.163.sslip.io/v1/public/token \\",
      "  -H 'Content-Type: application/x-www-form-urlencoded' \\",
      "  --data-urlencode 'client_id=local-client' \\",
      "  --data-urlencode 'client_secret=local-secret' \\",
      "  --data-urlencode 'grant_type=client_credentials' \\",
      "  --data-urlencode 'scope=ride.trips'",
    ].join("\n"),
    language: "bash",
  },
  webhooks: {
    title: "Webhooks",
    description: "Nhận thông báo khi trạng thái chuyến đi thay đổi",
    paragraphs: [
      "Webhooks gửi event khi trạng thái chuyến hoặc điểm dừng thay đổi. " +
        "Event được lưu trong PostgreSQL trước khi chuyển tới receiver.",
      "Thông báo có thể trùng hoặc đến khác thứ tự; dùng event_id để đối chiếu. " +
        "Hiện chưa hỗ trợ driver.location.updated.",
    ],
    code: JSON.stringify(
      {
        event_id: "<uuid>",
        event_time: 1770350401,
        event_type: "order_status_changed",
        resource_href: "https://gsm-api.103.176.113.163.sslip.io/v1/ride/orders/<id>",
        data: { order_id: "<id>", status: "ASSIGNED" },
        meta: { resource_id: "<id>", status: "ASSIGNED", user_id: "local-client" },
      },
      null,
      2,
    ),
    language: "json",
  },
  "green-connect": {
    title: "Green Connect",
    description: "Trung chuyển khách giữa bến xe và điểm đến",
    paragraphs: [
      "Chuyến trung chuyển gồm một PICK UP và ít nhất một DROP OFF. " +
        "Dùng payment_method=partner_pay và partner_payment_method=B2B.",
      "Mỗi điểm cần address_note theo mẫu Đón khách [Tên] - [SĐT] hoặc Trả khách [Tên] - [SĐT]. " +
        "partner_trip_id giúp đối chiếu với hệ thống của đối tác.",
    ],
    code: JSON.stringify(
      {
        payment_method: "partner_pay",
        partner_payment_method: "B2B",
        partner_trip_id: "BUS-TRIP-001",
        points: points.map((point, index) => ({
          ...point,
          address_note: (index === 0 ? "Đón khách " : "Trả khách ") + "Nguyễn Văn A - 0900000000",
        })),
      },
      null,
      2,
    ),
    language: "json",
  },
};
