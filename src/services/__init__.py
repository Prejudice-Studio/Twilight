"""
业务服务层

提供核心业务逻辑，所有 Emby 操作都通过 API 进行
"""
from src.services.emby import (
    EmbyClient,
    EmbyUser,
    EmbyLibrary,
    EmbySession,
    EmbyItem,
    EmbyError,
    EmbyAuthError,
    EmbyNotFoundError,
    EmbyConnectionError,
    get_emby_client,
    close_emby_client,
)
from src.services.emby_service import (
    EmbyService,
    EmbyUserStatus,
)
from src.services.user_service import (
    UserService,
    RegisterResult,
    RegisterResponse,
)
from src.services.score_service import (
    ScoreService,
    RedPacketService,
    CheckinResult,
    CheckinResponse,
    RedPacketType,
    RedPacketStatus,
)
from src.services.tmdb import (
    TMDBClient,
    TMDBMedia,
    TMDBError,
    get_tmdb_client,
    close_tmdb_client,
)
from src.services.bangumi import (
    BangumiClient,
    BangumiSubject,
    BangumiEpisode,
    BangumiError,
    SubjectType,
    EpStatus,
    get_bangumi_client,
    close_bangumi_client,
)
from src.services.bangumi_search import (
    BangumiSearchAPI,
    SubjectInfo as BangumiSubjectInfo,
)
from src.services.bangumi_sync import (
    BangumiSyncService,
    SyncRequest,
    SyncResult,
)
from src.services.media_service import (
    MediaService,
    MediaRequestService,
    MediaSource,
    MediaSearchResult,
    InventoryService,
    InventoryCheckResult,
)
from src.services.stats_service import (
    StatsService,
)
from src.services.webhook import (
    WebhookService,
    WebhookPushService,
    WebhookEvent,
    WebhookPayload,
)
from src.services.notification import (
    NotificationService,
    NotificationType,
    Notification,
)
from src.services.security_service import (
    SecurityService,
    LoginCheckResult,
    LoginCheckResponse,
)
from src.services.admin_service import (
    BatchOperationService,
    DataExportService,
    WatchHistoryService,
    ReminderService,
)
from src.services.auto_renew_service import (
    AutoRenewService,
)

__all__ = [
    # Emby API 客户端
    'EmbyClient',
    'EmbyUser',
    'EmbyLibrary',
    'EmbySession',
    'EmbyItem',
    'EmbyError',
    'EmbyAuthError',
    'EmbyNotFoundError',
    'EmbyConnectionError',
    'get_emby_client',
    'close_emby_client',
    # Emby 业务服务
    'EmbyService',
    'EmbyUserStatus',
    # 用户服务
    'UserService',
    'RegisterResult',
    'RegisterResponse',
    # 积分服务
    'ScoreService',
    'RedPacketService',
    'CheckinResult',
    'CheckinResponse',
    'RedPacketType',
    'RedPacketStatus',
    # TMDB
    'TMDBClient',
    'TMDBMedia',
    'TMDBError',
    'get_tmdb_client',
    'close_tmdb_client',
    # Bangumi
    'BangumiClient',
    'BangumiSubject',
    'BangumiEpisode',
    'BangumiError',
    'SubjectType',
    'EpStatus',
    'get_bangumi_client',
    'close_bangumi_client',
    # Bangumi 同步服务
    'BangumiSyncService',
    'SyncRequest',
    'SyncResult',
    # 媒体搜索
    'MediaService',
    'MediaRequestService',
    'MediaSource',
    'MediaSearchResult',
    # 库存检查
    'InventoryService',
    'InventoryCheckResult',
    # 统计服务
    'StatsService',
    # Webhook 服务
    'WebhookService',
    'WebhookPushService',
    'WebhookEvent',
    'WebhookPayload',
    # 通知服务
    'NotificationService',
    'NotificationType',
    'Notification',
    # 安全服务
    'SecurityService',
    'LoginCheckResult',
    'LoginCheckResponse',
    # 管理服务
    'BatchOperationService',
    'DataExportService',
    'WatchHistoryService',
    'ReminderService',
    # 自动续期
    'AutoRenewService',
]
