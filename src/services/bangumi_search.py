"""
Bangumi 智能搜索模块

基于用户提供的高级搜索逻辑，支持:
- 季度匹配
- 分割放送检测
- 置信度评分
- 多策略搜索
"""
import re
import logging
import asyncio
from dataclasses import dataclass
from typing import Optional, List, Tuple
from datetime import datetime
from difflib import SequenceMatcher

import httpx

from src.config import Config

logger = logging.getLogger(__name__)

BASE_URL = "https://api.bgm.tv/v0"


@dataclass
class SubjectInfo:
    """Bangumi 条目详细信息"""
    id: int
    type: int
    name: str
    name_cn: str
    name_en: str
    summary: str
    date: str
    eps: int
    volumes: int
    rating_score: float
    rating_rank: int
    rating_total: int
    tags: List[dict]
    nsfw: bool
    image: dict
    aliases: List[str]

    def get_romaji_alias(self) -> Optional[str]:
        """从别名列表中尝试找到罗马音名称"""
        for alias in self.aliases:
            if alias and alias.isascii():
                return alias.strip()
        if self.name_en and self.name_en.isascii():
            return self.name_en.strip()
        return None
    
    @property
    def title(self) -> str:
        """优先返回中文名"""
        return self.name_cn if self.name_cn else self.name
    
    @property
    def cover_url(self) -> Optional[str]:
        if self.image:
            return self.image.get('large') or self.image.get('common') or self.image.get('medium')
        return None
    
    @property
    def bgm_url(self) -> str:
        return f"https://bgm.tv/subject/{self.id}"
    
    def to_dict(self) -> dict:
        return {
            'id': self.id,
            'title': self.title,
            'original_title': self.name,
            'type': self.type,
            'overview': self.summary[:300] + '...' if len(self.summary) > 300 else self.summary,
            'release_date': self.date,
            'year': self.date[:4] if self.date and len(self.date) >= 4 else None,
            'poster_url': self.cover_url,
            'vote_average': self.rating_score,
            'rank': self.rating_rank,
            'bgm_url': self.bgm_url,
            'eps': self.eps,
            'nsfw': self.nsfw,
            'tags': [t.get('name') for t in self.tags[:5]] if self.tags else [],
            'source': 'bangumi',
        }


class BangumiSearchAPI:
    """Bangumi 智能搜索 API"""
    
    HEADERS = {
        "User-Agent": "Twilight/1.0 (Emby-Management)",
        "Content-Type": "application/json",
    }

    CONFIDENCE_THRESHOLD = 80
    PLATFORM_SUFFIX_PATTERN = re.compile(
        r"\s*[（\(](?:僅限.*地區|.*配|.*版|.*語)[）\)]\s*$"
    )
    SEASON_KEYWORD_SWAPS = {"第二季": "新系列", "第2季": "新系列"}

    SEASON_NUMBER_MAP = {
        "第一季": 1, "第1季": 1, "season 1": 1, "s1": 1, "1st season": 1, "Ⅰ": 1, "壹": 1, "一期": 1,
        "第二季": 2, "第2季": 2, "season 2": 2, "s2": 2, "2nd season": 2, "Ⅱ": 2, "贰": 2, "二期": 2,
        "第三季": 3, "第3季": 3, "season 3": 3, "s3": 3, "3rd season": 3, "Ⅲ": 3, "叁": 3, "三期": 3,
        "第四季": 4, "第4季": 4, "season 4": 4, "s4": 4, "4th season": 4, "Ⅳ": 4, "肆": 4, "四期": 4,
        "第五季": 5, "第5季": 5, "season 5": 5, "s5": 5, "5th season": 5, "Ⅴ": 5, "伍": 5, "五期": 5,
        "第6季": 6, "season 6": 6, "s6": 6, "6th season": 6, "Ⅵ": 6, "陆": 6, "六期": 6,
        "第7季": 7, "season 7": 7, "s7": 7, "7th season": 7, "Ⅶ": 7, "柒": 7, "七期": 7,
        "第8季": 8, "season 8": 8, "s8": 8, "8th season": 8, "Ⅷ": 8, "捌": 8, "八期": 8,
        "第9季": 9, "season 9": 9, "s9": 9, "9th season": 9, "Ⅸ": 9, "玖": 9, "九期": 9,
        "第10季": 10, "season 10": 10, "s10": 10, "10th season": 10, "Ⅹ": 10, "拾": 10, "十期": 10,
        "第六季": 6, "第七季": 7, "第八季": 8, "第九季": 9, "第十季": 10,
    }

    NON_NUMERIC_SEASON_KEYWORDS = [
        "新系列", "最终季", "最终章", "final season",
        "第二部分", "第2部分", "part 2", "part2",
    ]

    SEASON_PATTERNS = list(SEASON_NUMBER_MAP.keys()) + NON_NUMERIC_SEASON_KEYWORDS

    # --- 基础 API 请求 ---
    @staticmethod
    async def _get(endpoint: str, retries: int = 3, delay: int = 2) -> Optional[dict]:
        url = f"{BASE_URL}{endpoint}"
        async with httpx.AsyncClient() as client:
            for attempt in range(retries):
                try:
                    response = await client.get(
                        url, headers=BangumiSearchAPI.HEADERS, timeout=30
                    )
                    response.raise_for_status()
                    return response.json()
                except httpx.RequestError as e:
                    logger.warning(f"GET请求失败 (Attempt {attempt + 1}/{retries}): {e}")
                    if attempt < retries - 1:
                        await asyncio.sleep(delay)
            logger.error(f"GET请求在 {retries} 次尝试后最终失败: {endpoint}")
            return None

    @staticmethod
    async def _post(
            endpoint: str, data: dict, retries: int = 3, delay: int = 2
    ) -> Optional[dict]:
        url = f"{BASE_URL}{endpoint}"
        async with httpx.AsyncClient() as client:
            for attempt in range(retries):
                try:
                    response = await client.post(
                        url, json=data, headers=BangumiSearchAPI.HEADERS, timeout=30
                    )
                    response.raise_for_status()
                    return response.json()
                except httpx.RequestError as e:
                    logger.warning(f"POST请求失败 (Attempt {attempt + 1}/{retries}): {e}")
                    if attempt < retries - 1:
                        await asyncio.sleep(delay)
            logger.error(f"POST请求在 {retries} 次尝试后最终失败: {endpoint}")
            return None

    @classmethod
    async def get_subject_info(cls, subject_id: int) -> Optional[SubjectInfo]:
        """获取条目详细信息"""
        data = await cls._get(f"/subjects/{subject_id}")
        if not data:
            return None
        
        aliases = []
        infobox = data.get("infobox", [])
        if isinstance(infobox, list):
            for item in infobox:
                if item.get("key") == "别名":
                    value = item.get("value")
                    if isinstance(value, str):
                        aliases.extend([
                            alias.strip()
                            for alias in re.split(r"[、/]", value)
                            if alias.strip()
                        ])
                    elif isinstance(value, list):
                        for alias_obj in value:
                            if isinstance(alias_obj, dict) and "v" in alias_obj:
                                aliases.append(alias_obj["v"])
        
        return SubjectInfo(
            id=data.get("id"),
            type=data.get("type"),
            name=data.get("name", ""),
            name_cn=data.get("name_cn", ""),
            name_en=data.get("name_en", ""),
            summary=data.get("summary", ""),
            date=data.get("date", ""),
            eps=data.get("eps", 0),
            volumes=data.get("volumes", 0),
            rating_score=data.get("rating", {}).get("score", 0.0),
            rating_rank=data.get("rating", {}).get("rank", 0),
            rating_total=data.get("rating", {}).get("total", 0),
            tags=data.get("tags", []),
            nsfw=data.get("nsfw", False),
            image=data.get("images", {}),
            aliases=aliases,
        )

    # --- 独立工具函数 ---
    @classmethod
    async def get_poster(cls, subject_id: int) -> Optional[str]:
        """获取指定番剧的海报链接"""
        subject = await cls.get_subject_info(subject_id)
        if subject and subject.image:
            return subject.image.get("large") or subject.image.get("common") or subject.image.get("medium")
        return None

    @classmethod
    async def get_summary(cls, subject_id: int) -> Optional[str]:
        """获取指定番剧的简介"""
        subject = await cls.get_subject_info(subject_id)
        return subject.summary if subject else None

    @classmethod
    async def get_name(cls, subject_id: int, prefer_cn: bool = True) -> Optional[str]:
        """获取指定番剧的名称"""
        subject = await cls.get_subject_info(subject_id)
        if not subject:
            return None
        if prefer_cn:
            return subject.name_cn or subject.name
        return subject.name or subject.name_cn

    @classmethod
    async def get_total_episodes(cls, subject_id: int) -> int:
        """获取指定番剧的总集数"""
        subject = await cls.get_subject_info(subject_id)
        return subject.eps if subject else 0

    @classmethod
    async def get_all_episodes(cls, subject_id: int) -> List[dict]:
        """获取指定番剧的所有集数信息"""
        data = await cls._get(f"/subjects/{subject_id}/ep")
        return data.get("data", []) if data else []

    @classmethod
    async def get_episode_title(cls, subject_id: int, episode_number: int) -> Optional[str]:
        """获取指定番剧特定集数的标题"""
        episodes = await cls.get_all_episodes(subject_id)
        for ep in episodes:
            if ep.get("sort") == episode_number:
                return ep.get("name_cn") or ep.get("name")
        return None

    @classmethod
    async def get_related_subjects(cls, subject_id: int) -> List[dict]:
        """获取指定番剧的关联条目 (续集、前传等)"""
        data = await cls._get(f"/subjects/{subject_id}/relations")
        return data if data else []

    # --- 核心搜索与匹配逻辑 ---
    @classmethod
    async def search_subject(
            cls, name: str, subject_type: int = 2, episode_to_sync: int = 0
    ) -> Optional[SubjectInfo]:
        """
        智能搜索条目
        
        :param name: 搜索名称
        :param subject_type: 条目类型 (2=动画, 6=三次元)
        :param episode_to_sync: 需要同步的集数（用于分割放送检测）
        """
        initial_clean_name = cls.PLATFORM_SUFFIX_PATTERN.sub("", name).strip()
        if initial_clean_name != name:
            logger.info(f"清理标题: '{name}' -> '{initial_clean_name}'")

        logger.info(f"开始搜索: '{initial_clean_name}', 类型: {subject_type}")

        strategies = [(initial_clean_name, "直接搜索")]
        
        # 关键词替换策略
        for orig, new in cls.SEASON_KEYWORD_SWAPS.items():
            if orig in initial_clean_name:
                strategies.append(
                    (initial_clean_name.replace(orig, new), "关键词替换搜索")
                )

        # 基础名称搜索策略
        base_name = cls._normalize_title_for_comparison(initial_clean_name)
        if base_name and base_name != initial_clean_name.lower().replace(" ", ""):
            strategies.append((base_name, "基础名称搜索"))

        for term, s_name in strategies:
            logger.debug(f"--- {s_name}: '{term}' ---")
            results = await cls.search_subject_list(term, subject_type, 10)
            match_subject = await cls._find_match_in_results(
                results, initial_clean_name, None, episode_to_sync
            )
            if match_subject:
                return match_subject

        # 三次元分类搜索
        if subject_type != 6:
            logger.debug(f"--- 三次元分类搜索: '{initial_clean_name}' ---")
            results = await cls.search_subject_list(initial_clean_name, 6, 10)
            match_subject = await cls._find_match_in_results(
                results, initial_clean_name, None, episode_to_sync, is_3d_search=True
            )
            if match_subject:
                return match_subject

        logger.info(f"经过所有搜索策略仍未找到 '{initial_clean_name}' 的匹配项")
        return None

    @classmethod
    def _extract_season_number(cls, title: str) -> int:
        """提取季度编号"""
        if not title:
            return 0
        cleaned_title = cls.PLATFORM_SUFFIX_PATTERN.sub("", title).strip()
        title_lower = cleaned_title.lower()
        
        for keyword, number in sorted(
                cls.SEASON_NUMBER_MAP.items(), key=lambda item: len(item[0]), reverse=True
        ):
            if keyword in title_lower:
                return number
        
        match = re.search(r"\s+(\d+)$", cleaned_title)
        if match:
            season_num = int(match.group(1))
            if 1 < season_num < 20:
                return season_num
        return 0

    @classmethod
    def _normalize_title_for_comparison(cls, title: str) -> str:
        """标准化标题用于比较"""
        if not title:
            return ""
        normalized = cls.PLATFORM_SUFFIX_PATTERN.sub("", title).strip()
        normalized = normalized.lower()
        # 移除所有标点符号和空格（包括中文标点）
        normalized = re.sub(r'[\s!"#$%&\'()*+,\-./:;<=>?@\[\\\]^_`{|}~\u00b7\u2018\u2019\u201c\u201d\uff0c\u3002\uff1f\uff01\u300a\u300b\u2014\u2026\uffe5\u3001\uff08\uff09\u3010\u3011\u300c\u300d]', '', normalized)
        
        # 移除季数关键词
        all_patterns = [p for p in cls.SEASON_PATTERNS if not p.startswith("\\b")]
        for p in sorted(all_patterns, key=len, reverse=True):
            normalized = normalized.replace(p.lower(), "")

        normalized = re.sub(r"\s+(\d+)$", "", normalized)
        normalized = re.sub(r"\b(i|ii|iii|iv|v)\b", "", normalized, flags=re.IGNORECASE)
        return normalized.strip()

    @classmethod
    def _parse_bgm_date(cls, date_str: str) -> Optional[datetime]:
        """解析 Bangumi 日期格式"""
        if not date_str:
            return None
        try:
            if len(date_str) == 10:
                return datetime.strptime(date_str, "%Y-%m-%d")
            if len(date_str) == 7:
                return datetime.strptime(date_str + "-01", "%Y-%m-%d")
            if len(date_str) == 4:
                return datetime.strptime(date_str + "-01-01", "%Y-%m-%d")
        except ValueError:
            return None

    @classmethod
    def _calculate_confidence_score(
            cls,
            subject: SubjectInfo,
            search_name: str,
            search_date: Optional[datetime],
            rank: int,
    ) -> Tuple[int, List[str]]:
        """计算置信度分数"""
        score, details = 0, []
        norm_search = cls._normalize_title_for_comparison(search_name)
        names_to_check = [subject.name_cn, subject.name] + subject.aliases

        # 1. 名称相似度
        max_sim = 0
        best_match_name = ""
        for name in names_to_check:
            if not name:
                continue
            norm_subject_name = cls._normalize_title_for_comparison(name)
            sim = SequenceMatcher(None, norm_search, norm_subject_name).ratio()
            if sim > max_sim:
                max_sim = sim
                best_match_name = name
        
        name_score = int(max_sim * 100)
        score += name_score
        details.append(f"名称相似度({max_sim:.2f}): {name_score}分")

        # 2. 季数匹配度
        search_season_num = cls._extract_season_number(search_name)
        search_has_non_numeric = any(
            keyword.lower() in search_name.lower() for keyword in cls.NON_NUMERIC_SEASON_KEYWORDS)

        result_season_num = cls._extract_season_number(best_match_name) if best_match_name else 0
        result_has_non_numeric = any(
            keyword.lower() in best_match_name.lower() for keyword in cls.NON_NUMERIC_SEASON_KEYWORDS
        ) if best_match_name else False

        has_search_season_info = search_season_num > 0 or search_has_non_numeric
        has_result_season_info = result_season_num > 0 or result_has_non_numeric

        if has_search_season_info and has_result_season_info:
            if search_season_num > 0 and search_season_num == result_season_num:
                score += 50
                details.append(f"季数精确匹配(S{search_season_num}): +50分")
            elif search_season_num > 0 and 0 < result_season_num != search_season_num:
                score -= 30
                details.append(f"季数不匹配(S{search_season_num} vs S{result_season_num}): -30分")
            else:
                score += 40
                details.append("季数信息兼容: +40分")
        elif has_search_season_info != has_result_season_info:
            if has_search_season_info and not has_result_season_info:
                score -= 30
                details.append("季数不匹配(搜有/结果无): -30分")
            else:
                score -= 10
                details.append("季数潜在匹配(搜无/结果有): -10分")
        else:
            score += 25
            details.append("季数一致(均无): +25分")

        # 3. 日期匹配度
        if search_date and subject.date:
            bgm_date = cls._parse_bgm_date(subject.date)
            if bgm_date:
                delta = abs((search_date - bgm_date).days)
                if delta <= 31:
                    score += 30
                    details.append("日期匹配(≤1个月): +30分")
                elif delta <= 92:
                    score += 24
                    details.append("日期匹配(≤1个季度): +24分")
                elif delta <= 183:
                    score += 16
                    details.append("日期匹配(≤半年): +16分")
                elif delta <= 366:
                    score += 8
                    details.append("日期匹配(≤1年): +8分")

        # 4. 搜索排名加分
        if rank == 0:
            score += 15
            details.append(f"搜索排名(#{rank + 1}): +15分")
        elif rank <= 2:
            score += 10
            details.append(f"搜索排名(#{rank + 1}): +10分")

        return score, details

    @classmethod
    async def _find_match_in_results(
            cls,
            results_list: List[dict],
            original_search_name: str,
            search_date: Optional[datetime],
            episode_to_sync: int = 0,
            is_3d_search: bool = False,
    ) -> Optional[SubjectInfo]:
        """在搜索结果中找到最佳匹配"""
        if not results_list:
            return None

        candidates = []
        for i, result in enumerate(results_list):
            subject = await cls.get_subject_info(result["id"])
            if not subject or (0 < subject.eps < 6 and not is_3d_search):
                continue
            
            score, details = cls._calculate_confidence_score(
                subject, original_search_name, search_date, i
            )
            result_name = subject.name_cn or subject.name
            logger.debug(
                f"评估: '{result_name}' (ID: {subject.id}, 集数: {subject.eps}) | 总分: {score} | "
                + " | ".join(details)
            )
            
            if score >= cls.CONFIDENCE_THRESHOLD:
                candidates.append((score, subject))
            elif not is_3d_search:
                norm_search = cls._normalize_title_for_comparison(original_search_name)
                norm_subject_name = cls._normalize_title_for_comparison(result_name)
                sim = SequenceMatcher(None, norm_search, norm_subject_name).ratio()
                if sim > 0.75 and subject.eps > 0 and episode_to_sync > subject.eps:
                    candidates.append((score, subject))

        if not candidates:
            return None

        candidates.sort(key=lambda x: x[0], reverse=True)
        tried_subject_ids = set()

        for i, (score, current_subject) in enumerate(candidates):
            if current_subject.id in tried_subject_ids:
                continue

            result_name = current_subject.name_cn or current_subject.name
            
            # 检查是否为分割放送的第一部分
            is_potential_part1 = False
            if current_subject.eps > 0 and episode_to_sync > current_subject.eps:
                is_potential_part1 = True
            elif i == 0 and score >= cls.CONFIDENCE_THRESHOLD:
                is_potential_part1 = True

            if is_potential_part1:
                # 寻找后续部分
                for j, (next_score, next_subject) in enumerate(candidates):
                    if next_subject.id in tried_subject_ids or next_subject.id == current_subject.id:
                        continue

                    next_result_name = next_subject.name_cn or next_subject.name
                    norm_current_name = cls._normalize_title_for_comparison(result_name)
                    norm_next_name = cls._normalize_title_for_comparison(next_result_name)
                    name_sim_ratio = SequenceMatcher(None, norm_current_name, norm_next_name).ratio()

                    current_season_num = cls._extract_season_number(result_name)
                    next_season_num = cls._extract_season_number(next_result_name)

                    is_reasonable_continuation = False
                    if current_season_num > 0 and next_season_num > 0 and next_season_num == current_season_num + 1:
                        is_reasonable_continuation = True
                    elif current_season_num == 1 and next_season_num == 0:
                        is_reasonable_continuation = True
                    elif current_season_num == 0 and next_season_num == 2:
                        is_reasonable_continuation = True

                    if name_sim_ratio > 0.85 and is_reasonable_continuation:
                        adjusted_episode_to_sync = episode_to_sync - current_subject.eps
                        if next_subject.eps == 0 or adjusted_episode_to_sync <= next_subject.eps:
                            if adjusted_episode_to_sync > 0:
                                logger.info(f"匹配成功 (分割放送): '{next_result_name}' (ID: {next_subject.id})")
                                return next_subject

                        tried_subject_ids.add(next_subject.id)
                        break

            # 直接匹配
            if (current_subject.eps == 0 or episode_to_sync <= current_subject.eps) and score >= cls.CONFIDENCE_THRESHOLD:
                search_season_num = cls._extract_season_number(original_search_name)
                result_season_num = cls._extract_season_number(result_name)
                if search_season_num > 0 and 0 < result_season_num != search_season_num:
                    tried_subject_ids.add(current_subject.id)
                    continue

                logger.info(f"匹配成功: '{result_name}' (ID: {current_subject.id}), 置信度: {score}")
                return current_subject

            tried_subject_ids.add(current_subject.id)

        return None

    @classmethod
    async def search_subject_list(
            cls, name: str, subject_type: int = 2, limit: int = 5
    ) -> List[dict]:
        """搜索条目列表"""
        sanitized_name = re.sub(r'[,?!()\[\]"\'。，、？！（）【】「」]', ' ', name).strip()

        data = await cls._post(
            "/search/subjects",
            {
                "keyword": sanitized_name,
                "sort": "rank",
                "filter": {"type": [subject_type], "nsfw": True},
            },
        )
        
        if data and "data" in data and data["data"]:
            results = cls._process_subjects(data["data"], limit)
            logger.debug(f"使用关键词 '{sanitized_name}' 找到 {len(results)} 个结果")
            return results
        return []

    @classmethod
    def _process_subjects(cls, subjects: List[dict], limit: int) -> List[dict]:
        """处理搜索结果"""
        results = []
        for subject in subjects[:limit]:
            id_ = subject.get("id", 0)
            name = subject.get("name_cn", "").strip() or subject.get("name", "").strip()
            if id_ and name:
                results.append({"name": name, "id": id_})
        return results
    
    # --- 简单搜索方法（供外部调用）---
    @classmethod
    async def search(cls, keyword: str, subject_type: int = 2, limit: int = 20) -> List[SubjectInfo]:
        """
        简单搜索（返回所有匹配结果）
        
        :param keyword: 搜索关键词
        :param subject_type: 条目类型 (2=动画, 6=三次元)
        :param limit: 返回数量限制
        """
        results = await cls.search_subject_list(keyword, subject_type, limit)
        subjects = []
        for r in results:
            subject = await cls.get_subject_info(r['id'])
            if subject:
                subjects.append(subject)
        return subjects
    
    @classmethod
    async def smart_search(cls, keyword: str, subject_type: int = 2) -> Optional[SubjectInfo]:
        """
        智能搜索（返回最佳匹配）
        
        :param keyword: 搜索关键词
        :param subject_type: 条目类型 (2=动画, 6=三次元)
        """
        return await cls.search_subject(keyword, subject_type)

