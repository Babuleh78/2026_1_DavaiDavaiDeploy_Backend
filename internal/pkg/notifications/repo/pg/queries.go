package pg

import _ "embed"

//go:embed sql/create_duel_notification.sql
var createDuelNotificationQuery string

//go:embed sql/get_notifications.sql
var getNotificationsQuery string

//go:embed sql/mark_notification_read.sql
var markNotificationReadQuery string

//go:embed sql/get_user_telegram_id.sql
var getUserTelegramIDQuery string
