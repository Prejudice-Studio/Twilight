import asyncio
import json
import uuid
import sys
import os

# Add project root to sys.path
sys.path.append(os.getcwd())

from src.db.bangumi import BangumiSessionFactory, BangumiRequireModel, TMDBRequireModel, ReqStatus
from sqlalchemy import text

async def migrate():
    async with BangumiSessionFactory() as session:
        # Check if old table 'require' exists
        try:
            result = await session.execute(text("SELECT count(*) FROM require"))
            count = result.scalar()
            print(f"Found {count} records in old table 'require'")
        except Exception as e:
            # Maybe it was already renamed?
            try:
                result = await session.execute(text("SELECT count(*) FROM require_old"))
                print("Found 'require_old' table. Migration might have already run.")
                return
            except:
                print(f"Old table 'require' not found: {e}")
                return

        # Fetch all records
        result = await session.execute(text("SELECT * FROM require"))
        records = result.fetchall()

        for record in records:
            # record: (id, telegram_id, bangumi_id, status, timestamp, other_info)
            # rid, telegram_id, bangumi_id, status, timestamp, other_info = record
            # Use indexed access to be safer
            telegram_id = record[1]
            bangumi_id = record[2]
            status = record[3]
            timestamp = record[4]
            other_info = record[5]
            
            other = {}
            if other_info:
                try:
                    other = json.loads(other_info)
                except:
                    pass
            
            source = other.get('source', 'bangumi')
            media_info = other.get('media_info', {})
            
            title = media_info.get('title') or "Unknown"
            season = other.get('season') or media_info.get('season')
            year = media_info.get('year') or (media_info.get('release_date', '')[:4] if media_info.get('release_date') else None)
            media_type = media_info.get('media_type', 'anime' if source == 'bangumi' else 'unknown')
            require_key = uuid.uuid4().hex
            
            if source == 'tmdb':
                new_req = TMDBRequireModel(
                    telegram_id=telegram_id,
                    tmdb_id=str(bangumi_id),
                    status=status,
                    timestamp=timestamp,
                    title=title,
                    season=season,
                    year=str(year) if year else None,
                    media_type=media_type,
                    require_key=require_key,
                    other_info=other_info
                )
            else:
                new_req = BangumiRequireModel(
                    telegram_id=telegram_id,
                    bangumi_id=int(bangumi_id),
                    status=status,
                    timestamp=timestamp,
                    title=title,
                    season=season,
                    year=str(year) if year else None,
                    media_type=media_type,
                    require_key=require_key,
                    other_info=other_info
                )
            
            session.add(new_req)
        
        await session.commit()
        
        # Rename old table
        await session.execute(text("ALTER TABLE require RENAME TO require_old"))
        await session.commit()
        
        print("Migration completed successfully.")

if __name__ == "__main__":
    asyncio.run(migrate())
