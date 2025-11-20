#ifndef CJIEBA_JIEBA_H
#define CJIEBA_JIEBA_H

#include <stdlib.h>
#include "util.h"

typedef void* Jieba;

// 核心结构体：用于 Go 侧零拷贝
typedef struct {
  size_t offset;
  size_t len;
} Word;

typedef enum {
  DefaultMode = 0,
  SearchMode = 1,
} TokenizeMode;

// 辅助结构体：用于 Extract
struct CWordWeight {
  char* word;
  double weight;
};

Jieba NewJieba(const char* dict_path,
      const char* hmm_path, 
      const char* user_dict,
      const char* idf_path,
      const char* stop_words_path);
void FreeJieba(Jieba);
void Trim();

// --- 修改点：Cut 和 CutForSearch 返回 Word* (Offset数组) ---
Word* Cut(Jieba handle, const char* sentence, int is_hmm_used);
Word* CutForSearch(Jieba handle, const char* sentence, int is_hmm_used);

// --- 保持原样：CutAll 返回字符串数组 ---
char** CutAll(Jieba handle, const char* sentence);

char** Tag(Jieba handle, const char* sentence);
void AddWord(Jieba handle, const char* word);
void AddWordEx(Jieba handle, const char* word, int freq, const char* tag);
void RemoveWord(Jieba handle, const char* word);

Word* Tokenize(Jieba x, const char* sentence, TokenizeMode mode, int is_hmm_used);

char** Extract(Jieba handle, const char* sentence, int top_k);
struct CWordWeight* ExtractWithWeight(Jieba handle, const char* sentence, int top_k);
void FreeWordWeights(struct CWordWeight* wws);

#endif // CJIEBA_JIEBA_H