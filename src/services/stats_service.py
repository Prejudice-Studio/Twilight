"""
统计服务

播放统计、排行榜等功能
参考: https://github.com/berry8838/Sakura_embyboss
"""
import time
import logging
from typing import List, Dict, Any, Optional
from datetime import datetime, timedelta

from src.db.playback import PlaybackOperate, DailyStatsOperate, PlaybackModel
from src.db.user import UserOperate
from src.core.utils import timestamp, format_duration

logger = logging.getLogger(__name__)


class StatsService:
    """统计服务"""
    
    @staticmethod
    def _get_date_str(ts: int = None) -> str:
        """获取日期字符串 YYYY-MM-DD"""
        dt = datetime.fromtimestamp(ts or timestamp())
        return dt.strftime('%Y-%m-%d')
    
    @staticmethod
    def _get_day_start(offset_days: int = 0) -> int:
        """获取指定天数偏移的当天开始时间戳"""
        now = datetime.now()
        target = now.replace(hour=0, minute=0, second=0, microsecond=0)
        target = target - timedelta(days=-offset_days)
        return int(target.timestamp())
    
    @staticmethod
    def _get_week_start() -> int:
        """获取本周开始时间戳（周一）"""
        now = datetime.now()
        monday = now - timedelta(days=now.weekday())
        monday = monday.replace(hour=0, minute=0, second=0, microsecond=0)
        return int(monday.timestamp())
    
    @staticmethod
    def _get_month_start() -> int:
        """获取本月开始时间戳"""
        now = datetime.now()
        first_day = now.replace(day=1, hour=0, minute=0, second=0, microsecond=0)
        return int(first_day.timestamp())
    
    @classmethod
    async def get_user_stats(cls, uid: int) -> Dict[str, Any]:
        """获取用户统计信息"""
        user = await UserOperate.get_user_by_uid(uid)
        if not user:
            return None
        
        total_duration = await PlaybackOperate.get_user_total_duration(uid)
        play_count = await PlaybackOperate.get_user_play_count(uid)
        
        # 今日统计
        today_start = cls._get_day_start()
        today_records = await PlaybackOperate.get_user_playback(uid, limit=1000)
        today_duration = sum(
            r.DURATION for r in today_records 
            if r.START_TIME >= today_start
        )
        today_count = sum(
            1 for r in today_records 
            if r.START_TIME >= today_start
        )
        
        return {
            'uid': uid,
            'username': user.USERNAME,
            'total': {
                'duration': total_duration,
                'duration_str': format_duration(total_duration),
                'play_count': play_count,
            },
            'today': {
                'duration': today_duration,
                'duration_str': format_duration(today_duration),
                'play_count': today_count,
            }
        }
    
    @classmethod
    async def get_ranking(
        cls,
        period: str = 'all',  # all, today, week, month
        by: str = 'duration',
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """
        获取排行榜
        
        :param period: 时间范围 (all=总榜, today=日榜, week=周榜, month=月榜)
        :param by: 排序方式 (duration=时长, count=次数)
        :param limit: 返回数量
        """
        start_time = None
        end_time = None
        
        if period == 'today':
            start_time = cls._get_day_start()
        elif period == 'week':
            start_time = cls._get_week_start()
        elif period == 'month':
            start_time = cls._get_month_start()
        
        ranking_data = await PlaybackOperate.get_play_ranking(
            start_time=start_time,
            end_time=end_time,
            limit=limit,
            by=by
        )
        
        # 填充用户信息
        results = []
        for i, item in enumerate(ranking_data, 1):
            user = await UserOperate.get_user_by_uid(item['uid'])
            
            if by == 'duration':
                value = item['total']
                value_str = format_duration(value)
            else:
                value = item['total']
                value_str = f"{value} 次"
            
            results.append({
                'rank': i,
                'uid': item['uid'],
                'username': user.USERNAME if user else '未知用户',
                'value': value,
                'value_str': value_str,
            })
        
        return results
    
    @classmethod
    async def get_media_ranking(
        cls,
        period: str = 'all',
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """获取媒体播放排行"""
        start_time = None
        
        if period == 'today':
            start_time = cls._get_day_start()
        elif period == 'week':
            start_time = cls._get_week_start()
        elif period == 'month':
            start_time = cls._get_month_start()
        
        return await PlaybackOperate.get_media_ranking(
            start_time=start_time,
            limit=limit
        )
    
    @classmethod
    async def get_recent_plays(cls, limit: int = 20) -> List[Dict[str, Any]]:
        """获取最近播放记录"""
        # 需要实现一个全局的播放记录查询
        # 暂时返回空列表
        return []
    
    @classmethod
    async def record_play_start(
        cls,
        uid: int,
        emby_user_id: str,
        item_id: str,
        item_name: str,
        item_type: str,
        client: str = None,
        device_name: str = None,
        play_method: str = None,
        series_name: str = None,
        season_name: str = None,
        ip_address: str = None
    ) -> int:
        """
        记录播放开始
        
        :return: 记录 ID
        """
        record = PlaybackModel(
            UID=uid,
            EMBY_USER_ID=emby_user_id,
            ITEM_ID=item_id,
            ITEM_NAME=item_name,
            ITEM_TYPE=item_type,
            SERIES_NAME=series_name,
            SEASON_NAME=season_name,
            PLAY_METHOD=play_method,
            CLIENT=client,
            DEVICE_NAME=device_name,
            START_TIME=timestamp(),
            IP_ADDRESS=ip_address,
        )
        
        await PlaybackOperate.add_playback(record)
        logger.info(f"播放开始: {item_name} (用户: {uid})")
        
        return record.ID
    
    @classmethod
    async def record_play_stop(
        cls,
        emby_user_id: str,
        item_id: str,
        position_ticks: int = None
    ) -> bool:
        """记录播放停止"""
        # 查找活跃会话
        record = await PlaybackOperate.get_active_session(emby_user_id, item_id)
        if not record:
            return False
        
        end_time = timestamp()
        duration = end_time - record.START_TIME
        
        await PlaybackOperate.update_playback(
            record.ID,
            END_TIME=end_time,
            DURATION=duration,
            POSITION_TICKS=position_ticks
        )
        
        # 更新每日统计
        date_str = cls._get_date_str(record.START_TIME)
        await DailyStatsOperate.update_daily_stats(
            date=date_str,
            uid=record.UID,
            play_count=1,
            duration=duration,
            items=1
        )
        
        logger.info(f"播放结束: {record.ITEM_NAME} (时长: {format_duration(duration)})")
        return True
    
    @classmethod
    async def record_play_progress(
        cls,
        emby_user_id: str,
        item_id: str,
        position_ticks: int,
        is_paused: bool = False
    ) -> bool:
        """记录播放进度"""
        record = await PlaybackOperate.get_active_session(emby_user_id, item_id)
        if not record:
            return False
        
        await PlaybackOperate.update_playback(
            record.ID,
            POSITION_TICKS=position_ticks,
            IS_PAUSED=is_paused
        )
        
        return True

