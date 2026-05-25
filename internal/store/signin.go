package store

import "time"

const signinDateLayout = "2006-01-02"

func (s *Store) Signin(uid int64) Signin {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Signin[uid]
}

func (s *Store) AddSignin(uid int64, points int) (Signin, bool, error) {
	return s.AddSigninWithOptions(uid, points, nil, true)
}

func (s *Store) AddSigninWithOptions(uid int64, dailyPoints int, bonusForStreak func(int) int, resetAfterMiss bool) (Signin, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.refreshLocked(); err != nil {
		return Signin{}, false, err
	}
	now := time.Now()
	today := now.Format(signinDateLayout)
	yesterday := now.AddDate(0, 0, -1).Format(signinDateLayout)
	si := s.state.Signin[uid]
	if si.UID == 0 {
		si.UID = uid
	}
	if si.LongestStreak < si.Streak {
		si.LongestStreak = si.Streak
	}
	if si.LastSignin == today {
		return si, false, nil
	}
	if si.LastSignin == yesterday {
		si.Streak++
	} else if si.LastSignin != "" && !resetAfterMiss {
		si.Streak++
	} else {
		si.Streak = 1
	}
	if si.Streak > si.LongestStreak {
		si.LongestStreak = si.Streak
	}
	bonusPoints := 0
	if bonusForStreak != nil {
		bonusPoints = bonusForStreak(si.Streak)
	}
	totalPoints := dailyPoints + bonusPoints
	si.LastSignin = today
	si.Points += totalPoints
	si.Records = append(si.Records, SigninRecord{Date: today, Points: dailyPoints, BonusPoints: bonusPoints, Total: totalPoints, Streak: si.Streak, CreatedAt: now.Unix()})
	s.state.Signin[uid] = si
	return si, true, s.saveLocked()
}
