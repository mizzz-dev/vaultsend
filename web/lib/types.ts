export type AuthUser = {
  id: string;
  email: string;
  display_name?: string;
  status: string;
  email_verified_at?: string;
  created_at: string;
};

export type AuthResponse = {
  user: AuthUser;
};

export type ShipmentListItem = {
  id: string;
  subject: string;
  share_mode: "url_shared" | "recipient_restricted" | string;
  status: string;
  created_at: string;
  expires_at: string;
  download_count: number;
  max_download_count: number;
  file_count: number;
};

export type ShipmentListResponse = {
  items: ShipmentListItem[];
  limit: number;
  offset: number;
  total: number;
};

export type ShipmentFile = {
  id: string;
  file_name: string;
  size: number;
};

export type ShipmentRecipient = {
  id: string;
  email: string;
  status: string;
};

export type ShipmentRecipientSummary = {
  recipient_id: string;
  email: string;
  recipient_status: string;
  notification_count: number;
  last_notification_status?: string;
  last_notification_type?: string;
  last_notified_at?: string;
  first_download_at?: string;
  last_download_at?: string;
  download_count: number;
  has_downloaded: boolean;
};

export type ShipmentDetail = {
  id: string;
  status: string;
  share_mode: string;
  subject: string;
  message?: string;
  expires_at: string;
  max_download_count: number;
  download_count: number;
  last_download_at?: string;
  files: ShipmentFile[];
  recipients: ShipmentRecipient[];
  notification_summary: {
    total_notifications: number;
    queued_count: number;
    sent_count: number;
    failed_count: number;
    last_notification_at?: string;
  };
  recipient_summaries: ShipmentRecipientSummary[];
};

export type ApiErrorPayload = {
  error?: string;
  code?: string;
  message?: string;
  request_id?: string;
  upgrade_required?: boolean;
  upgrade_url?: string;
  recommended_plan?: string;
};
